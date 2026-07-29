package server

import (
	"fmt"
	"time"

	"ctf/internal/game"
	"ctf/internal/protocol"
)

// Este archivo expone el estado del servidor de solo lectura, para que el
// anfitrión pueda VER la partida sin tener que conectarse como jugador.
//
// El servidor no juega (§4), así que no puede mirar la partida con un cliente
// más: eso agregaría un jugador al mapa. En vez de eso, la interfaz del
// anfitrión (internal/serverui) llama a Vista() una vez por cuadro y dibuja lo
// que devuelve. Es una copia: se puede leer desde la goroutine de la interfaz
// mientras el ciclo de juego sigue corriendo en la suya.

// maxEventos es cuántas líneas del registro se conservan para la interfaz.
const maxEventos = 14

// VistaJugador es un jugador tal como lo ve el observador.
type VistaJugador struct {
	ID           uint16
	Nombre       string
	X, Y         float64
	Direccion    uint8
	LlevaBandera bool
}

// Vista es la foto completa del servidor en un instante: quién está conectado,
// dónde está cada uno, cómo va la bandera y qué pasó hace poco.
type Vista struct {
	Nombre     string // nombre del servidor
	Addr       string // dirección TCP donde escucha
	GameID     uint16
	Estado     uint8 // WAITING, STARTING, RUNNING, FINISHED
	Countdown  uint8 // segundos que faltan, si Estado es STARTING
	Tick       uint32
	Cfg        game.Config
	MaxPlayers int
	AutoStart  int
	Jugadores  []VistaJugador
	Bandera    game.Flag
	GanadorID  uint16
	Ganador    string
	Eventos    []string // los más recientes al final
}

// Vista devuelve una copia del estado actual, segura para cualquier goroutine.
func (s *Server) Vista() Vista {
	s.mu.Lock()
	v := Vista{
		Nombre:     s.opt.ServerName,
		GameID:     s.opt.GameID,
		Estado:     s.state,
		Countdown:  s.countdown,
		Tick:       s.world.Tick,
		Cfg:        s.opt.Config,
		MaxPlayers: s.opt.MaxPlayers,
		AutoStart:  s.opt.AutoStart,
		Bandera:    s.world.Flag,
		GanadorID:  s.ganadorID,
		Ganador:    s.ganador,
	}
	if s.ln != nil {
		v.Addr = s.ln.Addr().String()
	}
	for _, p := range s.world.Players() {
		v.Jugadores = append(v.Jugadores, VistaJugador{
			ID: p.ID, Nombre: p.Name, X: p.X, Y: p.Y,
			Direccion: p.Direction, LlevaBandera: s.world.HasFlag(p.ID),
		})
	}
	s.mu.Unlock()

	s.evMu.Lock()
	v.Eventos = append([]string(nil), s.eventos...)
	s.evMu.Unlock()
	return v
}

// PuedeIniciar indica si tiene sentido apretar el botón de inicio ahora mismo:
// solo desde el lobby y con al menos un jugador. La interfaz lo usa para dibujar
// el botón apagado en vez de dejar que el anfitrión arranque una partida vacía.
func (s *Server) PuedeIniciar() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state == protocol.StateWaiting && len(s.clients) > 0
}

// evento registra un hecho: lo escribe en el log de siempre y además lo guarda
// para la interfaz. Se usa solo para lo que vale la pena mostrar en pantalla;
// el resto sigue yendo por s.log directamente.
func (s *Server) evento(format string, a ...any) {
	linea := fmt.Sprintf(format, a...)
	s.log.Print(linea)

	s.evMu.Lock()
	s.eventos = append(s.eventos, time.Now().Format("15:04:05")+"  "+linea)
	if len(s.eventos) > maxEventos {
		s.eventos = s.eventos[len(s.eventos)-maxEventos:]
	}
	s.evMu.Unlock()
}
