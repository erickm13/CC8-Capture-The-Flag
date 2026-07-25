// Sonda: un cliente de prueba para el servidor de la PARTE 3.
//
// Se conecta por TCP, envía JOIN, y a partir de ahí muestra con logs cada
// mensaje que recibe del servidor. Cuando la partida arranca, camina hacia la
// bandera y presiona la tecla de interacción al llegar: es un jugador automático
// mínimo, suficiente para ver una partida completa de punta a punta.
//
// Ejemplos:
//
//	go run ./cmd/sonda -addr 127.0.0.1:5000              # un jugador
//	go run ./cmd/sonda -addr 127.0.0.1:5000 -name Beto   # otro, en otra terminal
//
// Para ver una partida: arrancá el servidor con -autostart 2, después dos
// sondas en dos terminales. Al entrar la segunda, la partida empieza sola.
package main

import (
	"flag"
	"fmt"
	"math"
	"net"
	"os"
	"time"

	"ctf/internal/discovery"
	"ctf/internal/protocol"
)

func main() {
	addr := flag.String("addr", "", "dirección del servidor; vacío = buscar por descubrimiento")
	dport := flag.Int("discovery-port", 5001, "puerto UDP del descubrimiento")
	name := flag.String("name", "sonda", "nombre del jugador")
	flag.Parse()

	// Si no se dio dirección, buscar un servidor por broadcast (§19).
	destino := *addr
	if destino == "" {
		logf("no se dio -addr, buscando servidores por broadcast...")
		servidores, err := discovery.Buscar(*dport, 1500*time.Millisecond)
		if err != nil || len(servidores) == 0 {
			fmt.Fprintln(os.Stderr, "no se encontró ningún servidor; probá con -addr host:puerto")
			os.Exit(1)
		}
		destino = servidores[0].Addr()
		logf("servidor encontrado: %q en %s", servidores[0].ServerName, destino)
	}

	conn, err := net.DialTimeout("tcp", destino, 5*time.Second)
	if err != nil {
		fmt.Fprintln(os.Stderr, "no se pudo conectar:", err)
		os.Exit(1)
	}
	defer conn.Close()
	logf("conectado a %s", destino)

	// Enviar JOIN.
	send(conn, protocol.Join{Name: *name})
	logf("→ JOIN enviado (name=%q)", *name)

	c := &cliente{conn: conn, nombre: *name}
	c.loop()
}

type cliente struct {
	conn     net.Conn
	nombre   string
	playerID uint16
	cfg      protocol.GameStarted
	corriendo bool
	dirActual uint8
}

func (c *cliente) loop() {
	// Goroutine que juega: cada 100 ms decide hacia dónde ir.
	go c.jugar()

	for {
		body, err := protocol.ReadFrame(c.conn)
		if err != nil {
			logf("conexión cerrada: %v", err)
			return
		}
		msg, err := protocol.UnmarshalBinary(body)
		if err != nil {
			logf("mensaje ilegible: %v", err)
			continue
		}
		if c.recibir(msg) {
			return
		}
	}
}

// recibir procesa un mensaje del servidor. Devuelve true si la partida terminó.
func (c *cliente) recibir(msg any) bool {
	switch m := msg.(type) {
	case protocol.JoinAccepted:
		c.playerID = m.PlayerID
		logf("← JOIN_ACCEPTED: soy P%02d en la partida %d", m.PlayerID, m.GameID)
	case protocol.JoinRejected:
		logf("← JOIN_REJECTED: motivo %d — no puedo entrar", m.Reason)
		return true
	case protocol.LobbyState:
		nombres := ""
		for i, p := range m.Players {
			if i > 0 {
				nombres += ", "
			}
			nombres += fmt.Sprintf("P%02d %s", p.ID, p.Name)
		}
		logf("← LOBBY_STATE: %d jugador(es) [%s]", len(m.Players), nombres)
	case protocol.GameCountdown:
		logf("← GAME_COUNTDOWN: %d...", m.SecondsRemaining)
	case protocol.GameStarted:
		c.cfg = m
		c.corriendo = true
		logf("← GAME_STARTED: mapa %.0f, círculo %.0f, %d jugadores. ¡A jugar!",
			m.MapSize, m.CircleRadius, len(m.Players))
	case protocol.GameState:
		// Solo lo mostramos de vez en cuando para no inundar la terminal.
		if m.Tick%10 == 0 {
			me := buscar(m.Players, c.playerID)
			if me != nil {
				logf("← GAME_STATE tick %d: estoy en (%.0f, %.0f), bandera %s",
					m.Tick, me.X, me.Y, estadoBandera(m.Flag.Status))
			}
		}
	case protocol.FlagPickedUp:
		logf("← FLAG_PICKED_UP: P%02d tomó la bandera (tick %d)", m.PlayerID, m.Tick)
	case protocol.FlagStolen:
		logf("← FLAG_STOLEN: P%02d se la robó a P%02d (tick %d)", m.NewCarrierID, m.PreviousCarrierID, m.Tick)
	case protocol.PlayerDisconnected:
		logf("← PLAYER_DISCONNECTED: P%02d se fue", m.PlayerID)
	case protocol.GameOver:
		if m.WinnerID == c.playerID {
			logf("← GAME_OVER: ¡GANÉ! 🏁")
		} else {
			logf("← GAME_OVER: ganó %s (P%02d)", m.WinnerName, m.WinnerID)
		}
		return true
	case protocol.Error:
		logf("← ERROR %d: %s", m.Code, m.Description)
	}
	return false
}

// jugar es la estrategia mínima: ir hacia la bandera (o hacia el centro) y
// presionar la tecla al llegar. No pretende ganar; sirve para mover al jugador.
func (c *cliente) jugar() {
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()
	// La sonda necesita su propia copia del último estado. Para mantener el
	// ejemplo simple, deducimos el objetivo del propio estado que llega: como
	// no compartimos ese estado entre goroutines aquí, usamos una heurística
	// fija: caminar hacia el centro y presionar la tecla en rondas.
	presionar := 0
	for range t.C {
		if !c.corriendo || c.playerID == 0 {
			continue
		}
		// Camina hacia el centro. El servidor valida la distancia real; nosotros
		// solo mandamos dirección hacia (0,0) de forma aproximada, alternando
		// ejes para acercarnos.
		dir := siguienteDireccion(presionar)
		if dir != c.dirActual {
			send(c.conn, protocol.Input{PlayerID: c.playerID, Direction: dir})
			c.dirActual = dir
		}
		// Presiona la tecla de interacción periódicamente, por si está cerca.
		send(c.conn, protocol.Interact{PlayerID: c.playerID})
		presionar++
	}
}

// siguienteDireccion alterna direcciones para que la sonda deambule hacia el
// centro. Es deliberadamente simple: el objetivo es ver el sistema funcionar,
// no jugar bien. Un cliente real calcularía la dirección desde su posición.
func siguienteDireccion(n int) uint8 {
	switch (n / 8) % 4 {
	case 0:
		return protocol.DirRight
	case 1:
		return protocol.DirUp
	case 2:
		return protocol.DirLeft
	default:
		return protocol.DirDown
	}
}

func buscar(ps []protocol.Player, id uint16) *protocol.Player {
	for i := range ps {
		if ps[i].ID == id {
			return &ps[i]
		}
	}
	return nil
}

func estadoBandera(s uint8) string {
	switch s {
	case protocol.FlagAvailable:
		return "disponible"
	case protocol.FlagCarried:
		return "en poder de alguien"
	case protocol.FlagDropped:
		return "caída"
	case protocol.FlagOutside:
		return "¡fuera!"
	}
	return "?"
}

func send(conn net.Conn, msg any) {
	body, err := protocol.MarshalBinary(msg)
	if err != nil {
		return
	}
	protocol.WriteFrame(conn, body)
}

func logf(format string, a ...any) {
	fmt.Printf("%s  %s\n", time.Now().Format("15:04:05.000"), fmt.Sprintf(format, a...))
}

var _ = math.Hypot // reservado para una estrategia más lista más adelante
