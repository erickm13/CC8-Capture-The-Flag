// probe: la prueba mínima de compatibilidad del §35 (PARTE 5).
//
// Ejecuta los pasos del §35 contra CUALQUIER servidor, sea de tu grupo o de
// otro, y dice si cumple el protocolo. Es la herramienta para verificar la
// interoperabilidad antes de la entrega: si el servidor de otro grupo pasa esta
// prueba, tu cliente va a poder jugar contra él.
//
// Ejemplos:
//
//	go run ./cmd/probe                        # descubre y prueba
//	go run ./cmd/probe -addr 192.168.1.40:5000  # contra otro grupo
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
	addr := flag.String("addr", "", "dirección del servidor; vacío = descubrir")
	dport := flag.Int("discovery-port", 5001, "puerto UDP del descubrimiento")
	name := flag.String("name", "sonda-§35", "nombre para el JOIN")
	debug := flag.Bool("debug", false, "mostrar cada mensaje en hex con desglose byte por byte")
	flag.Parse()

	protocol.DebugActivo = *debug

	fmt.Println("=== Prueba mínima de compatibilidad (§35) ===")
	fmt.Println()

	p := &prueba{limite: time.Now().Add(40 * time.Second)}

	// Paso 1: descubrimiento (o -addr).
	destino := *addr
	if destino == "" {
		servidores, err := discovery.Buscar(*dport, 2*time.Second)
		if err != nil || len(servidores) == 0 {
			p.falla(1, "Descubrimiento UDP", "ningún servidor respondió")
			p.reporte()
			os.Exit(1)
		}
		destino = servidores[0].Addr()
		p.ok(1, "Descubrimiento UDP", fmt.Sprintf("%q en %s", servidores[0].ServerName, destino))
	} else {
		p.omite(1, "Descubrimiento UDP", "se indicó -addr")
	}

	// Paso 2: conexión TCP.
	conn, err := net.DialTimeout("tcp", destino, 5*time.Second)
	if err != nil {
		p.falla(2, "Conexión TCP", err.Error())
		p.reporte()
		os.Exit(1)
	}
	defer conn.Close()
	p.ok(2, "Conexión TCP", destino)

	// Paso 3: JOIN → JOIN_ACCEPTED.
	enviar(conn, protocol.Join{Name: *name})
	var miID, gameID uint16
	switch m := p.esperar(conn, protocol.TypeJoinAccepted, protocol.TypeJoinRejected).(type) {
	case protocol.JoinAccepted:
		miID, gameID = m.PlayerID, m.GameID
		p.ok(3, "JOIN → JOIN_ACCEPTED", fmt.Sprintf("soy P%02d en la partida %d", miID, gameID))
	case protocol.JoinRejected:
		p.falla(3, "JOIN → JOIN_ACCEPTED", fmt.Sprintf("rechazado, motivo %d", m.Reason))
		p.reporte()
		os.Exit(1)
	default:
		p.falla(3, "JOIN → JOIN_ACCEPTED", "no llegó respuesta")
		p.reporte()
		os.Exit(1)
	}
	_ = gameID

	// Paso 4: LOBBY_STATE (puede no llegar si somos el único y arrancó ya).
	switch p.esperar(conn, protocol.TypeLobbyState, protocol.TypeGameCountdown, protocol.TypeGameStarted).(type) {
	case protocol.LobbyState:
		p.ok(4, "LOBBY_STATE", "lista de jugadores recibida")
	default:
		p.omite(4, "LOBBY_STATE", "la partida ya había arrancado")
	}

	// Paso 5: GAME_COUNTDOWN y GAME_STARTED.
	var cfg protocol.GameStarted
	if m, ok := p.esperar(conn, protocol.TypeGameStarted).(protocol.GameStarted); ok {
		cfg = m
		p.ok(5, "GAME_COUNTDOWN → GAME_STARTED",
			fmt.Sprintf("mapa %.0f, círculo %.0f, ciclo %d ms", m.MapSize, m.CircleRadius, m.TickIntervalMs))
	} else {
		p.falla(5, "GAME_COUNTDOWN → GAME_STARTED", "no llegó GAME_STARTED")
		p.reporte()
		os.Exit(1)
	}

	// Paso 6: INPUT mueve al jugador.
	primero := p.esperarEstado(conn)
	antes := buscar(primero.Players, miID)
	if antes == nil {
		p.falla(6, "INPUT mueve al jugador", "el estado no me incluye")
		p.reporte()
		os.Exit(1)
	}
	// Moverse hacia el centro (donde está la bandera).
	dir := direccionHacia(antes.X, antes.Y, 0, 0)
	enviar(conn, protocol.Input{PlayerID: miID, Direction: dir})

	movido := false
	var ultimo protocol.GameState
	for i := 0; i < 40 && !movido; i++ {
		st := p.esperarEstado(conn)
		ultimo = st
		if desp := buscar(st.Players, miID); desp != nil {
			if math.Hypot(desp.X-antes.X, desp.Y-antes.Y) > 0.5 {
				movido = true
			}
		}
	}
	if movido {
		p.ok(6, "INPUT mueve al jugador", "la posición cambió en el estado")
	} else {
		p.falla(6, "INPUT mueve al jugador", "la posición no cambió")
	}

	// Paso 7: INTERACT toma la bandera.
	tomada := false
	for i := 0; i < 400 && !tomada && time.Now().Before(p.limite); i++ {
		st := p.esperarEstado(conn)
		ultimo = st
		yo := buscar(st.Players, miID)
		if yo == nil {
			break
		}
		if yo.HasFlag {
			tomada = true
			break
		}
		if math.Hypot(st.Flag.X-yo.X, st.Flag.Y-yo.Y) <= cfg.InteractionRadius {
			enviar(conn, protocol.Interact{PlayerID: miID})
		} else {
			enviar(conn, protocol.Input{PlayerID: miID, Direction: direccionHacia(yo.X, yo.Y, st.Flag.X, st.Flag.Y)})
		}
	}
	if tomada {
		p.ok(7, "INTERACT toma la bandera", "hasFlag pasó a verdadero")
	} else {
		p.falla(7, "INTERACT toma la bandera", "no se logró tomar a tiempo")
	}

	// Paso 8: lectura de mensajes consecutivos.
	if ultimo.Tick > 3 {
		p.ok(8, "Mensajes consecutivos", fmt.Sprintf("%d ciclos leídos sin perder el hilo", ultimo.Tick))
	} else {
		p.falla(8, "Mensajes consecutivos", "se leyeron muy pocos")
	}

	// Paso 9: cierre limpio.
	enviar(conn, protocol.Leave{PlayerID: miID})
	conn.Close()
	p.ok(9, "Cierre de la conexión", "LEAVE enviado y socket cerrado")

	p.reporte()
	if p.fallas > 0 {
		os.Exit(1)
	}
}

// ---------- infraestructura de la prueba ----------

type prueba struct {
	pasos  int
	fallas int
	limite time.Time
}

func (p *prueba) linea(n int, titulo, estado, detalle string) {
	p.pasos++
	fmt.Printf("  %-6s %d. %-32s %s\n", estado, n, titulo, detalle)
}
func (p *prueba) ok(n int, t, d string)    { p.linea(n, t, "PASA", d) }
func (p *prueba) omite(n int, t, d string) { p.linea(n, t, "OMIT", d) }
func (p *prueba) falla(n int, t, d string) { p.fallas++; p.linea(n, t, "FALLA", d) }

func (p *prueba) reporte() {
	fmt.Printf("\n%d de %d comprobaciones superadas.\n", p.pasos-p.fallas, p.pasos)
	if p.fallas == 0 {
		fmt.Println("✓ El servidor cumple la prueba mínima de compatibilidad (§35).")
	} else {
		fmt.Println("✗ El servidor no cumple todavía. Revisar los pasos en FALLA.")
	}
}

// esperar lee hasta encontrar uno de los tipos indicados, descartando el resto.
func (p *prueba) esperar(conn net.Conn, tipos ...uint8) any {
	quiere := map[uint8]bool{}
	for _, t := range tipos {
		quiere[t] = true
	}
	for {
		if time.Now().After(p.limite) {
			return nil
		}
		conn.SetReadDeadline(time.Now().Add(15 * time.Second))
		body, err := protocol.ReadFrame(conn)
		if err != nil {
			return nil
		}
		protocol.LogRecibido("sonda", body)
		if len(body) < 1 || !quiere[body[0]] {
			continue
		}
		msg, err := protocol.UnmarshalBinary(body)
		if err != nil {
			continue
		}
		return msg
	}
}

func (p *prueba) esperarEstado(conn net.Conn) protocol.GameState {
	m := p.esperar(conn, protocol.TypeGameState)
	if st, ok := m.(protocol.GameState); ok {
		return st
	}
	return protocol.GameState{}
}

func buscar(ps []protocol.Player, id uint16) *protocol.Player {
	for i := range ps {
		if ps[i].ID == id {
			return &ps[i]
		}
	}
	return nil
}

func direccionHacia(x, y, objX, objY float64) uint8 {
	dx, dy := objX-x, objY-y
	if math.Abs(dx) >= math.Abs(dy) {
		if dx >= 0 {
			return protocol.DirRight
		}
		return protocol.DirLeft
	}
	if dy >= 0 {
		return protocol.DirDown
	}
	return protocol.DirUp
}

func enviar(conn net.Conn, msg any) {
	body, err := protocol.MarshalBinary(msg)
	if err != nil {
		return
	}
	protocol.LogEnviado("sonda", body)
	protocol.WriteFrame(conn, body)
}
