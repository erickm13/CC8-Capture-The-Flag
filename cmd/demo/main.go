// Programa de demostración de la PARTE 1.
//
// No es parte del juego: es una herramienta para que veas con tus ojos que la
// serialización binaria funciona. Serializa cada mensaje, imprime sus bytes en
// hexadecimal, los vuelve a leer y comprueba que coinciden con el original.
//
// Correlo así:
//
//	go run ./cmd/demo
//
// Lo que tenés que ver:
//   - El INPUT de P07 hacia arriba da exactamente "11 03 00 07 01" (la prueba
//     de oro del §35).
//   - Todos los mensajes dicen "ida y vuelta OK".
//   - El enmarcado TCP separa bien dos mensajes pegados.
package main

import (
	"bytes"
	"fmt"
	"reflect"

	"ctf/internal/protocol"
)

func main() {
	fmt.Println("=== PARTE 1: protocolo binario del PRFC-CC8-2026 v3 ===")
	fmt.Println()

	pruebaDeOro()
	fmt.Println()
	tablaDeMensajes()
	fmt.Println()
	pruebaDeEnmarcado()
	fmt.Println()
	pruebaDeErrores()
}

// pruebaDeOro es la comprobación del §35: el INPUT de P07 hacia arriba debe dar
// exactamente estos 5 bytes. Si no coinciden, no se interopera con nadie.
func pruebaDeOro() {
	fmt.Println("── Prueba de oro (§35): INPUT de P07 hacia arriba ──")
	msg := protocol.Input{PlayerID: 7, Direction: protocol.DirUp}
	body, _ := protocol.MarshalBinary(msg)

	esperado := []byte{0x11, 0x03, 0x00, 0x07, 0x01}
	fmt.Printf("   bytes obtenidos : %s\n", hex(body))
	fmt.Printf("   bytes esperados : %s\n", hex(esperado))
	if bytes.Equal(body, esperado) {
		fmt.Println("   ✓ COINCIDEN — este proyecto puede interoperar")
	} else {
		fmt.Println("   ✗ NO COINCIDEN — revisar el codec antes de seguir")
	}
	fmt.Println()
	fmt.Println("   desglose byte por byte:")
	fmt.Println("     11  → tipo INPUT")
	fmt.Println("     03  → versión 3")
	fmt.Println("     00 07 → playerId 7 (u16 big-endian)")
	fmt.Println("     01  → dirección UP")
}

// tablaDeMensajes serializa un ejemplo de cada mensaje, muestra sus bytes y
// verifica que sobrevive ida y vuelta (struct → bytes → struct).
func tablaDeMensajes() {
	fmt.Println("── Todos los mensajes: ida y vuelta ──")

	carrier := uint16(7)
	casos := []struct {
		nombre string
		msg    any
	}{
		{"DISCOVER_REQUEST", protocol.DiscoverRequest{}},
		{"DISCOVER_RESPONSE", protocol.DiscoverResponse{GameID: 1, ServerName: "Partida de Ana",
			TCPPort: 5000, State: protocol.StateWaiting, PlayerCount: 3, MaximumPlayers: 100}},
		{"JOIN", protocol.Join{Name: "Pepito"}},
		{"INPUT", protocol.Input{PlayerID: 7, Direction: protocol.DirLeft}},
		{"INTERACT", protocol.Interact{PlayerID: 7}},
		{"LEAVE", protocol.Leave{PlayerID: 7}},
		{"JOIN_ACCEPTED", protocol.JoinAccepted{PlayerID: 7, GameID: 1}},
		{"JOIN_REJECTED", protocol.JoinRejected{Reason: protocol.ReasonGameFull}},
		{"LOBBY_STATE", protocol.LobbyState{State: protocol.StateWaiting,
			Players: []protocol.LobbyPlayer{{ID: 1, Name: "Ana"}, {ID: 2, Name: "Beto"}}}},
		{"GAME_COUNTDOWN", protocol.GameCountdown{SecondsRemaining: 3}},
		{"GAME_STARTED", protocol.GameStarted{MapSize: 2000, CircleRadius: 500, PlayerRadius: 15,
			PlayerSpeed: 220, InteractionRadius: 60, TickIntervalMs: 50,
			Flag:    protocol.Flag{Status: protocol.FlagAvailable},
			Players: []protocol.Player{{ID: 1, Name: "Ana", X: -410.5, Y: -410.5, Direction: protocol.DirNone}}}},
		{"GAME_STATE", protocol.GameState{Tick: 185,
			Flag: protocol.Flag{Status: protocol.FlagCarried, X: 318.4, Y: -95.1, Carrier: carrier},
			Players: []protocol.Player{
				{ID: 1, X: -120.75, Y: 44.2, Direction: protocol.DirRight},
				{ID: 7, X: 318.4, Y: -95.1, Direction: protocol.DirLeft, HasFlag: true},
			}}},
		{"FLAG_PICKED_UP", protocol.FlagPickedUp{Tick: 90, PlayerID: 7}},
		{"FLAG_STOLEN", protocol.FlagStolen{Tick: 105, PreviousCarrierID: 1, NewCarrierID: 7}},
		{"PLAYER_DISCONNECTED", protocol.PlayerDisconnected{PlayerID: 7}},
		{"GAME_OVER", protocol.GameOver{WinnerID: 7, WinnerName: "Edgar", Reason: 0x01}},
		{"ERROR", protocol.Error{Code: protocol.ErrInvalidInput, Description: "dirección inválida"}},
	}

	fmt.Printf("   %-22s %6s   %s\n", "MENSAJE", "BYTES", "IDA Y VUELTA")
	for _, c := range casos {
		body, err := protocol.MarshalBinary(c.msg)
		if err != nil {
			fmt.Printf("   %-22s  ERROR al serializar: %v\n", c.nombre, err)
			continue
		}
		back, err := protocol.UnmarshalBinary(body)
		estado := "✓ OK"
		if err != nil {
			estado = "✗ error: " + err.Error()
		} else if !igualIgnorandoNombreCompacto(c.msg, back) {
			estado = "✗ el struct no volvió igual"
		}
		fmt.Printf("   %-22s %5d B   %s\n", c.nombre, len(body), estado)
	}
	fmt.Println()
	fmt.Println("   (GAME_STATE no incluye el nombre de cada jugador: el cliente")
	fmt.Println("    ya lo recibió en GAME_STARTED. Por eso pesa poco.)")
}

// pruebaDeEnmarcado demuestra §23.2: dos mensajes pegados en un flujo se separan
// por el prefijo de longitud, no por adivinanza.
func pruebaDeEnmarcado() {
	fmt.Println("── Enmarcado TCP (§23.2): dos mensajes pegados ──")

	m1, _ := protocol.MarshalBinary(protocol.Input{PlayerID: 1, Direction: protocol.DirRight})
	m2, _ := protocol.MarshalBinary(protocol.Interact{PlayerID: 1})

	var flujo bytes.Buffer
	protocol.WriteFrame(&flujo, m1)
	protocol.WriteFrame(&flujo, m2)

	fmt.Printf("   flujo en el cable: %s\n", hex(flujo.Bytes()))
	fmt.Println("   (los primeros 2 bytes 00 05 dicen 'vienen 5', luego el INPUT;")
	fmt.Println("    después 00 04 dice 'vienen 4', luego el INTERACT)")

	leido1, _ := protocol.ReadFrame(&flujo)
	leido2, _ := protocol.ReadFrame(&flujo)
	fmt.Printf("   mensaje 1 recuperado: %s  %s\n", hex(leido1), okSi(bytes.Equal(leido1, m1)))
	fmt.Printf("   mensaje 2 recuperado: %s  %s\n", hex(leido2), okSi(bytes.Equal(leido2, m2)))
}

// pruebaDeErrores comprueba que el codec rechaza mensajes malos en vez de
// romperse: versión incompatible, mensaje cortado, tipo desconocido.
func pruebaDeErrores() {
	fmt.Println("── Manejo de mensajes inválidos ──")

	// Versión 2 en vez de 3.
	_, err := protocol.UnmarshalBinary([]byte{0x11, 0x02, 0x00, 0x07, 0x01})
	fmt.Printf("   versión incompatible → %s\n", errTxt(err))

	// INPUT cortado: falta la dirección.
	_, err = protocol.UnmarshalBinary([]byte{0x11, 0x03, 0x00})
	fmt.Printf("   mensaje cortado      → %s\n", errTxt(err))

	// Tipo 0xFF que no existe.
	_, err = protocol.UnmarshalBinary([]byte{0xFF, 0x03})
	fmt.Printf("   tipo desconocido     → %s\n", errTxt(err))
}

// ---------- utilidades de impresión ----------

func hex(b []byte) string {
	s := ""
	for i, c := range b {
		if i > 0 {
			s += " "
		}
		s += fmt.Sprintf("%02X", c)
	}
	if s == "" {
		return "(vacío)"
	}
	return s
}

func okSi(b bool) string {
	if b {
		return "✓"
	}
	return "✗"
}

func errTxt(err error) string {
	if err == nil {
		return "✗ no dio error (debería haberlo dado)"
	}
	return "✓ rechazado: " + err.Error()
}

// igualIgnorandoNombreCompacto compara admitiendo que GAME_STATE pierde los
// nombres a propósito, y que las coordenadas se redondean a 2 decimales.
func igualIgnorandoNombreCompacto(orig, back any) bool {
	if gs, ok := orig.(protocol.GameState); ok {
		g2 := back.(protocol.GameState)
		if len(gs.Players) != len(g2.Players) {
			return false
		}
		for i := range gs.Players {
			a, b := gs.Players[i], g2.Players[i]
			if a.ID != b.ID || a.Direction != b.Direction || a.HasFlag != b.HasFlag {
				return false
			}
			if !cerca(a.X, b.X) || !cerca(a.Y, b.Y) {
				return false
			}
		}
		return gs.Tick == g2.Tick && gs.Flag.Status == g2.Flag.Status
	}
	// Para el resto, comparación directa tras redondear coordenadas.
	return reflect.DeepEqual(redondear(orig), redondear(back))
}

func redondear(v any) any {
	if gs, ok := v.(protocol.GameStarted); ok {
		for i := range gs.Players {
			gs.Players[i].X = round2(gs.Players[i].X)
			gs.Players[i].Y = round2(gs.Players[i].Y)
		}
		return gs
	}
	return v
}

func cerca(a, b float64) bool { return round2(a) == round2(b) }
func round2(v float64) float64 {
	if v < 0 {
		return float64(int64(v*100-0.5)) / 100
	}
	return float64(int64(v*100+0.5)) / 100
}
