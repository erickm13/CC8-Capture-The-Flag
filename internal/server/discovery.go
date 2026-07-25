package server

import (
	"fmt"
	"net"

	"ctf/internal/protocol"
)

// PARTE 4: descubrimiento por UDP (§19).
//
// El servidor escucha en un puerto UDP y responde los DISCOVER_REQUEST que
// lleguen por broadcast, pero solo mientras acepta jugadores (estado WAITING).
// Así un cliente encuentra las partidas de la red sin escribir la IP a mano.

// ListenDiscovery abre el socket UDP y arranca el bucle de respuesta. Se llama
// desde main después de Listen. Si el puerto está ocupado, devuelve el error
// pero el servidor puede seguir: la conexión manual por IP siempre funciona.
func (s *Server) ListenDiscovery(port int) error {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: port})
	if err != nil {
		return fmt.Errorf("puerto UDP %d ocupado: %w", port, err)
	}
	s.udp = conn
	s.discoveryPort = port
	go s.discoveryLoop()
	s.log.Printf("descubrimiento escuchando en UDP %d", port)
	return nil
}

func (s *Server) discoveryLoop() {
	buf := make([]byte, 2048)
	for {
		n, from, err := s.udp.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-s.done:
			default:
				s.log.Printf("descubrimiento: %v", err)
			}
			return
		}

		// ¿Es un DISCOVER_REQUEST válido? Si no, lo ignoramos sin responder.
		msg, derr := protocol.UnmarshalBinary(buf[:n])
		if derr != nil {
			continue
		}
		if _, ok := msg.(protocol.DiscoverRequest); !ok {
			continue
		}

		// §19: solo responden los servidores que aceptan jugadores.
		s.mu.Lock()
		waiting := s.state == protocol.StateWaiting
		resp := protocol.DiscoverResponse{
			GameID:         s.opt.GameID,
			ServerName:     s.opt.ServerName,
			TCPPort:        uint16(s.tcpPortLocked()),
			State:          s.state,
			PlayerCount:    uint16(len(s.clients)),
			MaximumPlayers: uint16(s.opt.MaxPlayers),
		}
		s.mu.Unlock()
		if !waiting {
			continue
		}

		body, err := protocol.MarshalBinary(resp)
		if err != nil {
			continue
		}
		// La respuesta va directa al remitente (from), no por broadcast. Su IP
		// sale del datagrama: un servidor con varias interfaces no sabe cuál ve
		// el cliente, por eso la dirección no viaja dentro del mensaje (§27.2).
		if _, err := s.udp.WriteToUDP(body, from); err != nil {
			s.log.Printf("respuesta de descubrimiento a %s: %v", from, err)
		} else {
			s.log.Printf("respondí un descubrimiento a %s", from.IP)
		}
	}
}

// tcpPortLocked devuelve el puerto TCP real (puede diferir si se pidió el 0).
func (s *Server) tcpPortLocked() int {
	if s.ln != nil {
		if a, ok := s.ln.Addr().(*net.TCPAddr); ok {
			return a.Port
		}
	}
	return s.opt.TCPPort
}
