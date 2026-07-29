// Package server implementa el modo servidor del PRFC-CC8-2026 v3.
//
// PARTE 3 de la implementación. Aquí el juego empieza a aceptar conexiones TCP
// reales. Toda la lógica del juego sigue en internal/game; este paquete solo
// agrega la red: aceptar clientes, el lobby, la cuenta regresiva, el ciclo en
// tiempo real y la difusión del estado.
//
// El servidor no juega (§4): no tiene entidad en el mapa, solo coordina y
// muestra. La máquina que lo corre observa; para jugar hace falta un cliente.
package server

import (
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"ctf/internal/game"
	"ctf/internal/protocol"
)

// Options configura el servidor.
type Options struct {
	GameID        uint16
	ServerName    string
	TCPPort       int
	Config        game.Config
	CountdownSecs int
	PostGameSecs  int // segundos que se muestra el GAME_OVER antes de volver al lobby
	MaxPlayers    int
	AutoStart     int // opcional: arrancar solo al llegar a N jugadores; 0 = manual (comando/botón start)
	Logger        *log.Logger
}

// client es una conexión aceptada. w serializa mensajes hacia ese cliente.
type client struct {
	conn     net.Conn
	playerID uint16
	name     string
	mu       sync.Mutex // protege la escritura al socket
}

func (c *client) send(msg any) error {
	body, err := protocol.MarshalBinary(msg)
	if err != nil {
		return err
	}
	protocol.LogEnviado("servidor", body)
	c.mu.Lock()
	defer c.mu.Unlock()
	return protocol.WriteFrame(c.conn, body)
}

// Server es una partida.
type Server struct {
	opt   Options
	log   *log.Logger
	mu    sync.Mutex
	state uint8
	world *game.World

	clients map[uint16]*client
	nextID  uint16

	pendingInteract map[uint16]bool

	// Estado extra que solo existe para el observador (ver vista.go). La
	// interfaz del anfitrión lo lee una vez por cuadro; vive bajo el mismo mutex
	// que el resto del estado del servidor.
	countdown uint8
	ganadorID uint16
	ganador   string

	// Registro de los últimos hechos de la partida, para mostrarlos en la
	// interfaz. Tiene su propio mutex porque se escribe desde tick(), que ya
	// tiene s.mu tomado.
	evMu    sync.Mutex
	eventos []string

	ln            net.Listener
	udp           *net.UDPConn
	discoveryPort int
	done          chan struct{}
	startCh       chan struct{}
	closeer       sync.Once
}

func New(opt Options) *Server {
	if opt.GameID == 0 {
		opt.GameID = 1
	}
	if opt.ServerName == "" {
		opt.ServerName = "Captura la Bandera"
	}
	if opt.TCPPort == 0 {
		opt.TCPPort = 5000
	}
	if opt.CountdownSecs == 0 {
		opt.CountdownSecs = 5
	}
	if opt.PostGameSecs == 0 {
		opt.PostGameSecs = 5
	}
	if opt.MaxPlayers == 0 {
		opt.MaxPlayers = 100
	}
	if opt.Config.MapSize == 0 {
		opt.Config = game.DefaultConfig()
	}
	if opt.Logger == nil {
		opt.Logger = log.Default()
	}
	return &Server{
		opt:             opt,
		log:             opt.Logger,
		state:           protocol.StateWaiting,
		world:           game.New(opt.Config, nil),
		clients:         map[uint16]*client{},
		pendingInteract: map[uint16]bool{},
		done:            make(chan struct{}),
		startCh:         make(chan struct{}, 1),
	}
}

// Addr devuelve la dirección TCP real (útil si se pidió el puerto 0).
func (s *Server) Addr() net.Addr { return s.ln.Addr() }

// Estado devuelve el estado actual del servidor (WAITING, RUNNING, etc.), de
// forma segura para otras goroutines.
func (s *Server) Estado() uint8 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Listen abre el puerto TCP sin empezar a atender.
func (s *Server) Listen() error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", s.opt.TCPPort))
	if err != nil {
		return fmt.Errorf("no se pudo abrir el puerto TCP %d: %w", s.opt.TCPPort, err)
	}
	s.ln = ln
	return nil
}

// Run atiende conexiones y ejecuta partidas en bucle: cuando una termina, el
// servidor vuelve al lobby y espera otro 'start' para la siguiente. Así se
// pueden jugar varias partidas sin reiniciar el proceso ni reconectar.
func (s *Server) Run() {
	go s.acceptLoop()
	s.log.Printf("servidor '%s' escuchando en %s (gameId %d)",
		s.opt.ServerName, s.ln.Addr(), s.opt.GameID)
	s.log.Printf("esperando jugadores...")

	for {
		// Esperar la orden de inicio. startCh tiene capacidad 1, así que un
		// 'start' escrito en cualquier momento (incluso durante la partida
		// anterior) queda encolado y arranca la siguiente en cuanto Run vuelve
		// a esperar aquí. No lo vaciamos en ningún lado: eso evitaba un arranque
		// de más, pero causaba que se perdiera el 'start' legítimo del anfitrión
		// al volver al lobby (una carrera de milisegundos). Es preferible que un
		// 'start' nunca se pierda a que ocasionalmente sobre uno.
		select {
		case <-s.startCh:
		case <-s.done:
			return
		}

		s.runCountdown()
		s.runGame()

		// La partida terminó. Si el servidor no se está cerrando, volvemos al
		// lobby para poder jugar otra.
		select {
		case <-s.done:
			return
		default:
		}

		// Pausa antes de volver al lobby: mantiene el estado FINISHED (y el
		// GAME_OVER en pantalla) un rato, para que TODOS los clientes alcancen a
		// mostrar quién ganó. Sin esta pausa, el LOBBY_STATE llegaría pegado al
		// GAME_OVER y algunos clientes borrarían el cartel de victoria antes de
		// que se viera. El protocolo no fija este tiempo, así que lo elegimos
		// generoso para máxima compatibilidad.
		s.log.Printf("mostrando el resultado por %d s...", s.opt.PostGameSecs)
		select {
		case <-time.After(time.Duration(s.opt.PostGameSecs) * time.Second):
		case <-s.done:
			return
		}

		s.volverAlLobby()
	}
}

// volverAlLobby resetea el mundo y deja el servidor listo para otra partida.
func (s *Server) volverAlLobby() {
	s.mu.Lock()
	s.world.Reiniciar()
	s.state = protocol.StateWaiting
	s.pendingInteract = map[uint16]bool{}
	s.countdown = 0
	s.ganadorID, s.ganador = 0, ""
	s.broadcastLobbyLocked()
	jugadores := len(s.clients)
	s.mu.Unlock()

	s.evento("de vuelta en el lobby con %d jugador(es). Escribí 'start' para otra partida.", jugadores)
}

// Start dispara el inicio de la partida (lo hace el anfitrión). Encola la señal
// en startCh; el bucle Run la recoge solo cuando está esperando en el lobby, así
// que un start mandado durante una partida en curso simplemente queda listo para
// la siguiente vez que Run vuelva a esperar (o se descarta si ya hay uno).
func (s *Server) Start() {
	select {
	case s.startCh <- struct{}{}:
	default:
		// Ya había una señal encolada: no hace falta otra.
	}
}

// Close cierra el servidor.
func (s *Server) Close() {
	s.closeer.Do(func() {
		close(s.done)
		s.ln.Close()
		if s.udp != nil {
			s.udp.Close()
		}
	})
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			select {
			case <-s.done:
			default:
				s.log.Printf("accept: %v", err)
			}
			return
		}
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()
	c := &client{conn: conn}
	s.log.Printf("conexión nueva desde %s", conn.RemoteAddr())

	for {
		body, err := protocol.ReadFrame(conn)
		if err != nil {
			break
		}
		protocol.LogRecibido("servidor", body)
		msg, derr := protocol.UnmarshalBinary(body)
		if derr != nil {
			c.send(protocol.Error{Code: errorCode(derr), Description: derr.Error()})
			if derr == protocol.ErrVersion {
				break // nada de lo que siga servirá
			}
			continue
		}
		if stop := s.dispatch(c, msg); stop {
			break
		}
	}
	s.disconnect(c)
}

func errorCode(err error) uint8 {
	switch err {
	case protocol.ErrVersion:
		return protocol.ErrUnsupportedVersion
	case protocol.ErrShort, protocol.ErrType:
		return protocol.ErrInvalidEncoding
	default:
		return protocol.ErrInvalidMessage
	}
}

// dispatch procesa un mensaje. Devuelve true si hay que cerrar la conexión.
func (s *Server) dispatch(c *client, msg any) bool {
	switch m := msg.(type) {
	case protocol.Join:
		return !s.handleJoin(c, m)
	case protocol.Input:
		s.handleInput(c, m)
	case protocol.Interact:
		s.handleInteract(c, m)
	case protocol.Leave:
		return true
	default:
		c.send(protocol.Error{Code: protocol.ErrInvalidMessage,
			Description: "este mensaje no va del cliente al servidor"})
	}
	return false
}

func (s *Server) handleJoin(c *client, m protocol.Join) bool {
	name := strings.TrimSpace(m.Name)
	if name == "" || len([]rune(name)) > 20 {
		c.send(protocol.JoinRejected{Reason: protocol.ReasonInvalidName})
		return false
	}

	s.mu.Lock()
	if s.state != protocol.StateWaiting {
		s.mu.Unlock()
		c.send(protocol.JoinRejected{Reason: protocol.ReasonGameAlreadyStarted})
		return false
	}
	if len(s.clients) >= s.opt.MaxPlayers {
		s.mu.Unlock()
		c.send(protocol.JoinRejected{Reason: protocol.ReasonGameFull})
		return false
	}
	s.nextID++
	id := s.nextID
	c.playerID, c.name = id, name
	s.clients[id] = c
	s.world.AddPlayer(id, name)
	count := len(s.clients)
	s.mu.Unlock()

	c.send(protocol.JoinAccepted{PlayerID: id, GameID: s.opt.GameID})
	s.mu.Lock()
	s.broadcastLobbyLocked()
	s.mu.Unlock()

	s.evento("P%02d entró como %q (%d jugador(es) en el lobby)", id, name, count)
	// AutoStart es opcional: si se configuró, la partida arranca sola al llegar
	// a N jugadores. Por defecto es 0, y la inicia el anfitrión con 'start'.
	if s.opt.AutoStart > 0 && count >= s.opt.AutoStart {
		s.log.Printf("se alcanzaron %d jugadores: iniciando automáticamente", s.opt.AutoStart)
		s.Start()
	}
	return true
}

func (s *Server) handleInput(c *client, m protocol.Input) {
	if c.playerID == 0 || c.playerID != m.PlayerID {
		c.send(protocol.Error{Code: protocol.ErrUnknownPlayer,
			Description: "el playerId no corresponde a esta conexión"})
		return
	}
	if m.Direction > protocol.DirRight {
		c.send(protocol.Error{Code: protocol.ErrInvalidInput,
			Description: "dirección fuera de rango"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == protocol.StateRunning {
		s.world.SetDirection(m.PlayerID, m.Direction)
	}
}

func (s *Server) handleInteract(c *client, m protocol.Interact) {
	if c.playerID == 0 || c.playerID != m.PlayerID {
		c.send(protocol.Error{Code: protocol.ErrUnknownPlayer,
			Description: "el playerId no corresponde a esta conexión"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == protocol.StateRunning {
		s.pendingInteract[m.PlayerID] = true // §12: varios cuentan como uno
	}
}

func (s *Server) disconnect(c *client) {
	if c.playerID == 0 {
		return
	}
	s.mu.Lock()
	delete(s.clients, c.playerID)
	s.world.RemovePlayer(c.playerID)
	delete(s.pendingInteract, c.playerID)
	s.broadcastLocked(protocol.PlayerDisconnected{PlayerID: c.playerID})
	if s.state == protocol.StateWaiting {
		s.broadcastLobbyLocked()
	}
	s.mu.Unlock()
	s.evento("P%02d se desconectó", c.playerID)
}

func (s *Server) runCountdown() {
	s.mu.Lock()
	s.state = protocol.StateStarting
	s.countdown = uint8(s.opt.CountdownSecs)
	s.mu.Unlock()
	s.evento("iniciando cuenta regresiva (%d s)", s.opt.CountdownSecs)

	for i := s.opt.CountdownSecs; i > 0; i-- {
		s.mu.Lock()
		s.countdown = uint8(i)
		s.broadcastLocked(protocol.GameCountdown{SecondsRemaining: uint8(i)})
		s.mu.Unlock()
		select {
		case <-time.After(time.Second):
		case <-s.done:
			return
		}
	}
}

func (s *Server) runGame() {
	s.mu.Lock()
	s.world.SpawnAll()
	s.state = protocol.StateRunning
	cfg := s.opt.Config
	s.broadcastLocked(protocol.GameStarted{
		MapSize: cfg.MapSize, CircleRadius: cfg.CircleRadius, PlayerRadius: cfg.PlayerRadius,
		PlayerSpeed: cfg.PlayerSpeed, InteractionRadius: cfg.InteractionRadius,
		TickIntervalMs: uint16(cfg.TickIntervalMs),
		Flag:           s.flagLocked(), Players: s.playersFullLocked(),
	})
	s.mu.Unlock()
	s.evento("¡partida iniciada! %d jugadores en el mapa", len(s.clients))

	ticker := time.NewTicker(time.Duration(cfg.TickIntervalMs) * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			if s.tick() {
				s.log.Printf("partida terminada")
				return
			}
		}
	}
}

// tick ejecuta un ciclo y difunde el resultado. Devuelve true si terminó.
func (s *Server) tick() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	interacts := s.pendingInteract
	s.pendingInteract = map[uint16]bool{}
	ev := s.world.Step(interacts)

	// §29.11: primero los eventos, después el estado.
	for _, id := range ev.PickedUp {
		s.broadcastLocked(protocol.FlagPickedUp{Tick: s.world.Tick, PlayerID: id})
		s.evento("tick %d: %s tomó la bandera", s.world.Tick, s.nombreLocked(id))
	}
	for _, st := range ev.Stolen {
		s.broadcastLocked(protocol.FlagStolen{Tick: s.world.Tick,
			PreviousCarrierID: st.Previous, NewCarrierID: st.New})
		s.evento("tick %d: %s le robó la bandera a %s",
			s.world.Tick, s.nombreLocked(st.New), s.nombreLocked(st.Previous))
	}
	s.broadcastLocked(protocol.GameState{
		Tick: s.world.Tick, Flag: s.flagLocked(), Players: s.playersCompactLocked(),
	})

	if ev.Winner != 0 {
		name := s.nombreLocked(ev.Winner)
		s.state = protocol.StateFinished
		s.ganadorID, s.ganador = ev.Winner, name
		s.broadcastLocked(protocol.GameOver{WinnerID: ev.Winner, WinnerName: name, Reason: 0x01})
		s.evento("tick %d: ¡ganó %s!", s.world.Tick, name)
		return true
	}
	return false
}

// nombreLocked devuelve el nombre de un jugador, o "P07" si ya no está (se pudo
// haber desconectado en el mismo ciclo). Asume el mutex tomado.
func (s *Server) nombreLocked(id uint16) string {
	if p := s.world.Player(id); p != nil && p.Name != "" {
		return p.Name
	}
	return fmt.Sprintf("P%02d", id)
}

// ---------- difusión (asumen el mutex tomado) ----------

func (s *Server) broadcastLocked(msg any) {
	for _, c := range s.clients {
		if err := c.send(msg); err != nil {
			s.log.Printf("no se pudo escribir a P%02d: %v", c.playerID, err)
		}
	}
}

func (s *Server) broadcastLobbyLocked() {
	players := make([]protocol.LobbyPlayer, 0, len(s.clients))
	for _, p := range s.world.Players() {
		players = append(players, protocol.LobbyPlayer{ID: p.ID, Name: p.Name})
	}
	s.broadcastLocked(protocol.LobbyState{State: s.state, Players: players})
}

func (s *Server) playersFullLocked() []protocol.Player {
	ps := s.world.Players()
	out := make([]protocol.Player, 0, len(ps))
	for _, p := range ps {
		out = append(out, protocol.Player{ID: p.ID, Name: p.Name, X: p.X, Y: p.Y,
			Direction: p.Direction, HasFlag: s.world.HasFlag(p.ID)})
	}
	return out
}

func (s *Server) playersCompactLocked() []protocol.Player {
	ps := s.world.Players()
	out := make([]protocol.Player, 0, len(ps))
	for _, p := range ps {
		out = append(out, protocol.Player{ID: p.ID, X: p.X, Y: p.Y,
			Direction: p.Direction, HasFlag: s.world.HasFlag(p.ID)})
	}
	return out
}

func (s *Server) flagLocked() protocol.Flag {
	f := s.world.Flag
	return protocol.Flag{Status: f.Status, X: f.X, Y: f.Y, Carrier: f.Carrier}
}
