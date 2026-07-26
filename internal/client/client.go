// Package client es un cliente reutilizable del PRFC-CC8-2026 v3.
//
// PARTE 5. A diferencia de la sonda (que era un ejemplo suelto), este cliente
// está pensado para que lo use cualquier capa de arriba: el bot automático de
// esta parte, y en la Parte 6 la interfaz gráfica con Ebitengine.
//
// La idea central: el cliente corre la red en su propia goroutine y mantiene el
// último estado recibido. Quien lo usa solo llama a Snapshot() para ver el
// estado actual y a Mover()/Interactuar() para actuar. No tiene que saber nada
// de bytes ni de sockets.
package client

import (
	"fmt"
	"net"
	"sync"
	"time"

	"ctf/internal/protocol"
)

// Snapshot es la foto del juego que ve quien usa el cliente. Es una copia: se
// puede leer desde cualquier goroutine sin candados.
type Snapshot struct {
	Conectado bool
	Estado    uint8
	MiID      uint16
	Config    protocol.GameStarted
	Jugadores []protocol.Player
	Bandera   protocol.Flag
	Tick      uint32
	Lobby     []protocol.LobbyPlayer
	Countdown uint8
	GanadorID uint16
	Ganador   string
	Nombres   map[uint16]string // playerId → nombre, recibidos en GAME_STARTED/LOBBY_STATE
}

// Nombre devuelve el nombre de un jugador por su ID. El GAME_STATE no trae
// nombres (§29.6), así que se buscan en la tabla que se llenó al recibir
// GAME_STARTED o LOBBY_STATE. Si no se conoce, cae en "P01" como respaldo.
func (s Snapshot) Nombre(id uint16) string {
	if s.Nombres != nil {
		if n, ok := s.Nombres[id]; ok && n != "" {
			return n
		}
	}
	return fmt.Sprintf("P%02d", id)
}

// Yo devuelve el jugador propio dentro del snapshot, o nil si no está.
func (s Snapshot) Yo() *protocol.Player {
	for i := range s.Jugadores {
		if s.Jugadores[i].ID == s.MiID {
			return &s.Jugadores[i]
		}
	}
	return nil
}

// Portador devuelve quién lleva la bandera, o nil si nadie.
func (s Snapshot) Portador() *protocol.Player {
	if s.Bandera.Carrier == 0 {
		return nil
	}
	for i := range s.Jugadores {
		if s.Jugadores[i].ID == s.Bandera.Carrier {
			return &s.Jugadores[i]
		}
	}
	return nil
}

// Eventos avisa de hechos puntuales, para sonido o animación. Todos opcionales.
type Eventos struct {
	AlTomar       func(tick uint32, id uint16)
	AlRobar       func(tick uint32, previo, nuevo uint16)
	AlDesconectar func(id uint16)
	AlTerminar    func(ganadorID uint16, nombre string)
}

// Cliente es una conexión a un servidor.
type Cliente struct {
	conn net.Conn
	ev   Eventos

	mu       sync.RWMutex
	snap     Snapshot
	lastTick uint32

	cerrado chan struct{}
	unaVez  sync.Once
}

// Conectar abre la conexión, envía JOIN y espera la respuesta del servidor.
func Conectar(addr, nombre string, ev Eventos) (*Cliente, error) {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("no se pudo conectar a %s: %w", addr, err)
	}
	c := &Cliente{conn: conn, ev: ev, cerrado: make(chan struct{})}
	c.snap.Conectado = true
	c.snap.Estado = protocol.StateWaiting

	if err := c.enviar(protocol.Join{Name: nombre}); err != nil {
		conn.Close()
		return nil, err
	}

	aceptado := make(chan error, 1)
	go c.leer(aceptado)

	select {
	case err := <-aceptado:
		if err != nil {
			c.Cerrar()
			return nil, err
		}
		return c, nil
	case <-time.After(5 * time.Second):
		c.Cerrar()
		return nil, fmt.Errorf("el servidor no respondió al JOIN")
	}
}

// Snapshot devuelve una copia del estado actual, segura para cualquier goroutine.
func (c *Cliente) Snapshot() Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s := c.snap
	s.Jugadores = append([]protocol.Player(nil), c.snap.Jugadores...)
	s.Lobby = append([]protocol.LobbyPlayer(nil), c.snap.Lobby...)
	// Copiar el mapa de nombres para que quien lea el snapshot no comparta el
	// mapa interno (que otra goroutine podría modificar).
	if c.snap.Nombres != nil {
		s.Nombres = make(map[uint16]string, len(c.snap.Nombres))
		for k, v := range c.snap.Nombres {
			s.Nombres[k] = v
		}
	}
	return s
}

// Mover envía la dirección activa. Conviene llamarlo solo cuando cambia: el
// servidor la mantiene vigente (§10).
func (c *Cliente) Mover(dir uint8) error {
	c.mu.RLock()
	id := c.snap.MiID
	c.mu.RUnlock()
	if id == 0 {
		return fmt.Errorf("todavía no tengo playerId")
	}
	return c.enviar(protocol.Input{PlayerID: id, Direction: dir})
}

// Interactuar avisa que se presionó la tecla de interacción (§12).
func (c *Cliente) Interactuar() error {
	c.mu.RLock()
	id := c.snap.MiID
	c.mu.RUnlock()
	if id == 0 {
		return fmt.Errorf("todavía no tengo playerId")
	}
	return c.enviar(protocol.Interact{PlayerID: id})
}

// Salir avisa que el jugador abandona y cierra la conexión.
func (c *Cliente) Salir() {
	c.mu.RLock()
	id := c.snap.MiID
	c.mu.RUnlock()
	if id != 0 {
		c.enviar(protocol.Leave{PlayerID: id})
	}
	c.Cerrar()
}

// Listo se cierra cuando la conexión termina, por cualquier motivo.
func (c *Cliente) Listo() <-chan struct{} { return c.cerrado }

func (c *Cliente) Cerrar() {
	c.unaVez.Do(func() {
		close(c.cerrado)
		c.conn.Close()
		c.mu.Lock()
		c.snap.Conectado = false
		c.mu.Unlock()
	})
}

func (c *Cliente) enviar(msg any) error {
	body, err := protocol.MarshalBinary(msg)
	if err != nil {
		return err
	}
	protocol.LogEnviado("cliente", body)
	return protocol.WriteFrame(c.conn, body)
}

// leer es el bucle de red: decodifica cada mensaje y actualiza el snapshot.
func (c *Cliente) leer(aceptado chan<- error) {
	defer c.Cerrar()
	joinResuelto := false

	for {
		body, err := protocol.ReadFrame(c.conn)
		if err != nil {
			if !joinResuelto {
				aceptado <- fmt.Errorf("se cerró la conexión antes de responder al JOIN")
			}
			return
		}
		protocol.LogRecibido("cliente", body)
		msg, err := protocol.UnmarshalBinary(body)
		if err != nil {
			continue // un mensaje ilegible no debe tumbar al cliente
		}

		switch m := msg.(type) {
		case protocol.JoinAccepted:
			c.mu.Lock()
			c.snap.MiID = m.PlayerID
			c.mu.Unlock()
			if !joinResuelto {
				joinResuelto = true
				aceptado <- nil
			}
		case protocol.JoinRejected:
			if !joinResuelto {
				joinResuelto = true
				aceptado <- fmt.Errorf("el servidor rechazó el JOIN (motivo %d)", m.Reason)
			}
			return
		case protocol.LobbyState:
			c.mu.Lock()
			c.snap.Estado, c.snap.Lobby = m.State, m.Players
			// Guardar los nombres por playerId, para dibujarlos durante la
			// partida (el GAME_STATE no los trae, §29.6).
			if c.snap.Nombres == nil {
				c.snap.Nombres = map[uint16]string{}
			}
			for _, p := range m.Players {
				c.snap.Nombres[p.ID] = p.Name
			}
			// Volver al lobby (por ejemplo tras una partida) limpia el resultado
			// y el estado de juego anterior, para que la interfaz muestre la
			// sala de espera sin residuos de la partida que terminó.
			c.snap.GanadorID, c.snap.Ganador = 0, ""
			c.snap.Tick = 0
			c.snap.Jugadores = nil
			c.lastTick = 0
			c.mu.Unlock()
		case protocol.GameCountdown:
			c.mu.Lock()
			c.snap.Estado, c.snap.Countdown = protocol.StateStarting, m.SecondsRemaining
			c.mu.Unlock()
		case protocol.GameStarted:
			c.mu.Lock()
			c.snap.Estado, c.snap.Config = protocol.StateRunning, m
			c.snap.Jugadores, c.snap.Bandera = m.Players, m.Flag
			// GAME_STARTED sí trae los nombres; los guardamos para el resto de
			// la partida, donde los GAME_STATE ya no los incluyen (§29.6).
			if c.snap.Nombres == nil {
				c.snap.Nombres = map[uint16]string{}
			}
			for _, p := range m.Players {
				c.snap.Nombres[p.ID] = p.Name
			}
			c.mu.Unlock()
			// Cada partida empieza en tick 1, así que reiniciamos el filtro de
			// estados viejos; si no, los estados de la nueva partida (ticks
			// bajos) se descartarían por ser menores al último de la anterior.
			c.lastTick = 0
		case protocol.GameState:
			if m.Tick <= c.lastTick { // §31: ignorar estados viejos
				continue
			}
			c.lastTick = m.Tick
			c.mu.Lock()
			c.snap.Tick, c.snap.Jugadores, c.snap.Bandera = m.Tick, m.Players, m.Flag
			c.mu.Unlock()
		case protocol.FlagPickedUp:
			if c.ev.AlTomar != nil {
				c.ev.AlTomar(m.Tick, m.PlayerID)
			}
		case protocol.FlagStolen:
			if c.ev.AlRobar != nil {
				c.ev.AlRobar(m.Tick, m.PreviousCarrierID, m.NewCarrierID)
			}
		case protocol.PlayerDisconnected:
			if c.ev.AlDesconectar != nil {
				c.ev.AlDesconectar(m.PlayerID)
			}
		case protocol.GameOver:
			c.mu.Lock()
			c.snap.Estado = protocol.StateFinished
			c.snap.GanadorID, c.snap.Ganador = m.WinnerID, m.WinnerName
			c.mu.Unlock()
			if c.ev.AlTerminar != nil {
				c.ev.AlTerminar(m.WinnerID, m.WinnerName)
			}
			// No cerramos: el servidor sigue vivo y nos mandará un LOBBY_STATE
			// para la próxima partida. Seguimos escuchando.
		}
	}
}
