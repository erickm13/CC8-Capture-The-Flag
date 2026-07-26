package protocol

import (
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// Desglosar toma los bytes de UN mensaje (sin el prefijo de longitud del
// enmarcado TCP) y devuelve un texto con el hex completo y el desglose campo por
// campo, según el tipo. Es la herramienta de depuración para ver exactamente qué
// viaja por el cable: quién manda qué bytes y cómo se interpretan.
//
// Se usa con el flag -debug del servidor y del cliente. El formato imita el
// desglose del PRFC (§23-§29): tipo, versión, y cada campo con su valor.
func Desglosar(body []byte) string {
	var b strings.Builder

	// Línea de hex, siempre.
	fmt.Fprintf(&b, "hex: %s\n", hexLinea(body))
	if len(body) < 2 {
		fmt.Fprintf(&b, "  (mensaje demasiado corto: %d byte(s))\n", len(body))
		return b.String()
	}

	tipo := body[0]
	ver := body[1]
	fmt.Fprintf(&b, "  %02X       -> tipo %s (0x%02X)\n", tipo, nombreTipo(tipo), tipo)
	fmt.Fprintf(&b, "  %02X       -> versión del protocolo (%d)\n", ver, ver)

	// Un lector para recorrer el cuerpo desde el byte 2 en adelante, anotando.
	d := &desglosador{b: body, i: 2, out: &b}
	switch tipo {
	case TypeDiscoverRequest:
		// solo encabezado
	case TypeDiscoverResponse:
		d.u16("gameId")
		d.str("serverName")
		d.u16("tcpPort")
		d.u8Estado("state")
		d.u16("playerCount")
		d.u16("maximumPlayers")
	case TypeJoin:
		d.str("name")
	case TypeInput:
		d.u16("playerId")
		d.u8Dir("direction")
	case TypeInteract, TypeLeave:
		d.u16("playerId")
	case TypeJoinAccepted:
		d.u16("playerId (asignado)")
		d.u16("gameId")
	case TypeJoinRejected:
		d.u8("reason")
	case TypeLobbyState:
		d.u8Estado("state")
		n := d.u8("count")
		for k := 0; k < n && d.ok(); k++ {
			fmt.Fprintf(&b, "  -- jugador %d --\n", k+1)
			d.u16("  playerId")
			d.str("  name")
		}
	case TypeGameCountdown:
		d.u8("secondsRemaining")
	case TypeGameStarted:
		d.i32("mapSize×100")
		d.i32("circleRadius×100")
		d.i32("playerRadius×100")
		d.i32("playerSpeed×100")
		d.i32("interactionRadius×100")
		d.u16("tickIntervalMs")
		d.u8Estado("flagStatus")
		d.u16("flagCarrierId")
		d.i32("flagX×100")
		d.i32("flagY×100")
		n := d.u8("count")
		for k := 0; k < n && d.ok(); k++ {
			fmt.Fprintf(&b, "  -- jugador %d --\n", k+1)
			d.u16("  playerId")
			d.str("  name")
			d.i32("  x×100")
			d.i32("  y×100")
			d.u8Dir("  direction")
			d.boolean("  hasFlag")
		}
	case TypeGameState:
		d.u32("tick")
		d.u8Estado("flagStatus")
		d.u16("flagCarrierId")
		d.i32("flagX×100")
		d.i32("flagY×100")
		n := d.u8("count")
		for k := 0; k < n && d.ok(); k++ {
			fmt.Fprintf(&b, "  -- jugador %d --\n", k+1)
			d.u16("  playerId")
			d.i32("  x×100")
			d.i32("  y×100")
			d.u8Dir("  direction")
			d.boolean("  hasFlag")
		}
	case TypeFlagPickedUp:
		d.u32("tick")
		d.u16("playerId")
	case TypeFlagStolen:
		d.u32("tick")
		d.u16("previousCarrierId")
		d.u16("newCarrierId")
	case TypePlayerDisconnect:
		d.u16("playerId")
	case TypeGameOver:
		d.u16("winnerId")
		d.str("winnerName")
		d.u8("reason")
	case TypeError:
		d.u8("code")
		d.str("description")
	default:
		fmt.Fprintf(&b, "  (tipo desconocido: no se puede desglosar el cuerpo)\n")
	}

	if d.i < len(body) {
		fmt.Fprintf(&b, "  sobran %d byte(s) sin interpretar: %s\n",
			len(body)-d.i, hexLinea(body[d.i:]))
	}
	return b.String()
}

// hexLinea formatea bytes como "29 03 00 01 ...".
func hexLinea(bs []byte) string {
	parts := make([]string, len(bs))
	for i, x := range bs {
		parts[i] = fmt.Sprintf("%02X", x)
	}
	return strings.Join(parts, " ")
}

func nombreTipo(t uint8) string {
	switch t {
	case TypeDiscoverRequest:
		return "DISCOVER_REQUEST"
	case TypeDiscoverResponse:
		return "DISCOVER_RESPONSE"
	case TypeJoin:
		return "JOIN"
	case TypeInput:
		return "INPUT"
	case TypeInteract:
		return "INTERACT"
	case TypeLeave:
		return "LEAVE"
	case TypeJoinAccepted:
		return "JOIN_ACCEPTED"
	case TypeJoinRejected:
		return "JOIN_REJECTED"
	case TypeLobbyState:
		return "LOBBY_STATE"
	case TypeGameCountdown:
		return "GAME_COUNTDOWN"
	case TypeGameStarted:
		return "GAME_STARTED"
	case TypeGameState:
		return "GAME_STATE"
	case TypeFlagPickedUp:
		return "FLAG_PICKED_UP"
	case TypeFlagStolen:
		return "FLAG_STOLEN"
	case TypePlayerDisconnect:
		return "PLAYER_DISCONNECTED"
	case TypeGameOver:
		return "GAME_OVER"
	case TypeError:
		return "ERROR"
	}
	return "?"
}

// desglosador recorre el cuerpo de un mensaje anotando cada campo que lee.
type desglosador struct {
	b   []byte
	i   int
	out *strings.Builder
}

func (d *desglosador) ok() bool { return d.i <= len(d.b) }

func (d *desglosador) u8(nombre string) int {
	if d.i+1 > len(d.b) {
		d.faltan(nombre)
		return 0
	}
	v := d.b[d.i]
	fmt.Fprintf(d.out, "  %02X       -> %s = %d\n", v, nombre, v)
	d.i++
	return int(v)
}

func (d *desglosador) u16(nombre string) {
	if d.i+2 > len(d.b) {
		d.faltan(nombre)
		return
	}
	v := binary.BigEndian.Uint16(d.b[d.i:])
	fmt.Fprintf(d.out, "  %02X %02X    -> %s = %d (u16 big-endian)\n", d.b[d.i], d.b[d.i+1], nombre, v)
	d.i += 2
}

func (d *desglosador) u32(nombre string) {
	if d.i+4 > len(d.b) {
		d.faltan(nombre)
		return
	}
	v := binary.BigEndian.Uint32(d.b[d.i:])
	fmt.Fprintf(d.out, "  %s -> %s = %d (u32 big-endian)\n",
		hexLinea(d.b[d.i:d.i+4]), nombre, v)
	d.i += 4
}

func (d *desglosador) i32(nombre string) {
	if d.i+4 > len(d.b) {
		d.faltan(nombre)
		return
	}
	v := int32(binary.BigEndian.Uint32(d.b[d.i:]))
	fmt.Fprintf(d.out, "  %s -> %s = %d (i32; ÷100 = %.2f)\n",
		hexLinea(d.b[d.i:d.i+4]), nombre, v, float64(v)/100)
	d.i += 4
}

func (d *desglosador) str(nombre string) {
	if d.i+1 > len(d.b) {
		d.faltan(nombre)
		return
	}
	n := int(d.b[d.i])
	fmt.Fprintf(d.out, "  %02X       -> %s: largo = %d (u8)\n", d.b[d.i], nombre, n)
	d.i++
	if d.i+n > len(d.b) {
		d.faltan(nombre + " (contenido)")
		return
	}
	s := string(d.b[d.i : d.i+n])
	if n > 0 {
		fmt.Fprintf(d.out, "  %s -> %s = %q\n", hexLinea(d.b[d.i:d.i+n]), nombre, s)
	} else {
		fmt.Fprintf(d.out, "           -> %s = \"\" (vacío)\n", nombre)
	}
	d.i += n
}

func (d *desglosador) boolean(nombre string) {
	if d.i+1 > len(d.b) {
		d.faltan(nombre)
		return
	}
	v := d.b[d.i] != 0
	fmt.Fprintf(d.out, "  %02X       -> %s = %v (bool)\n", d.b[d.i], nombre, v)
	d.i++
}

func (d *desglosador) u8Dir(nombre string) {
	if d.i+1 > len(d.b) {
		d.faltan(nombre)
		return
	}
	v := d.b[d.i]
	fmt.Fprintf(d.out, "  %02X       -> %s = %s (0x%02X)\n", v, nombre, nombreDir(v), v)
	d.i++
}

func (d *desglosador) u8Estado(nombre string) {
	if d.i+1 > len(d.b) {
		d.faltan(nombre)
		return
	}
	v := d.b[d.i]
	fmt.Fprintf(d.out, "  %02X       -> %s = 0x%02X\n", v, nombre, v)
	d.i++
}

func (d *desglosador) faltan(nombre string) {
	fmt.Fprintf(d.out, "  (faltan bytes para leer %s)\n", nombre)
	d.i = len(d.b) + 1 // marca que ya no se puede seguir
}

func nombreDir(v uint8) string {
	switch v {
	case DirNone:
		return "NONE"
	case DirUp:
		return "UP"
	case DirDown:
		return "DOWN"
	case DirLeft:
		return "LEFT"
	case DirRight:
		return "RIGHT"
	}
	return "?"
}

// --- activación del modo debug y guardado a archivo ---

// DebugActivo controla si los mensajes se muestran en pantalla. El servidor y el
// cliente lo ponen en true con -debug.
var DebugActivo = false

// GuardarActivo controla si además se escriben a un archivo. Se activa con -save.
var GuardarActivo = false

var (
	logMu      sync.Mutex
	logArchivo *os.File
	logQuien   string // "servidor" o "cliente", para nombrar los archivos
	logNumero  int    // número de partida en esta ejecución
)

// IniciarGuardado prepara el guardado a archivo. quien es el rol (servidor o
// cliente), que va en el nombre del archivo. No abre nada todavía: el primer
// archivo se crea cuando arranca la primera partida (NuevaPartidaLog).
func IniciarGuardado(quien string) {
	logMu.Lock()
	defer logMu.Unlock()
	GuardarActivo = true
	logQuien = quien
}

// NuevaPartidaLog cierra el archivo de la partida anterior (si había) y abre uno
// nuevo con la fecha y hora actual. Se llama cuando empieza una partida, para que
// cada una quede en su propio archivo. Si el guardado no está activo, no hace nada.
func NuevaPartidaLog() {
	logMu.Lock()
	defer logMu.Unlock()
	if !GuardarActivo {
		return
	}
	if logArchivo != nil {
		logArchivo.Close()
	}
	logNumero++
	// Nombre: logs/ctf-<quien>-<fecha>-<hora>-partidaNN.log
	os.MkdirAll("logs", 0o755)
	marca := time.Now().Format("2006-01-02_15-04-05")
	nombre := fmt.Sprintf("logs/ctf-%s-%s-partida%02d.log", logQuien, marca, logNumero)
	f, err := os.Create(nombre)
	if err != nil {
		fmt.Fprintf(os.Stderr, "no se pudo crear el log %s: %v\n", nombre, err)
		return
	}
	logArchivo = f
	fmt.Printf(">>> guardando el log de esta partida en %s\n", nombre)
	fmt.Fprintf(f, "=== Log de %s — partida %d — %s ===\n",
		logQuien, logNumero, time.Now().Format("2006-01-02 15:04:05"))
}

// CerrarGuardado cierra el archivo abierto, si hay. Se llama al terminar.
func CerrarGuardado() {
	logMu.Lock()
	defer logMu.Unlock()
	if logArchivo != nil {
		logArchivo.Close()
		logArchivo = nil
	}
}

// LogEnviado registra un mensaje que se está por enviar (flecha →).
func LogEnviado(quien string, body []byte) {
	registrar(quien, "→ ENVÍA", body)
}

// LogRecibido registra un mensaje recién recibido (flecha ←).
func LogRecibido(quien string, body []byte) {
	registrar(quien, "← RECIBE", body)
}

// registrar arma el texto del mensaje y lo manda a pantalla (si -debug) y al
// archivo (si -save). Si llega un GAME_STARTED y el guardado está activo, abre
// un archivo nuevo para esa partida antes de escribir.
func registrar(quien, flecha string, body []byte) {
	if !DebugActivo && !GuardarActivo {
		return
	}

	// Una partida nueva empieza con GAME_STARTED: abrimos archivo nuevo.
	if GuardarActivo && len(body) > 0 && body[0] == TypeGameStarted {
		NuevaPartidaLog()
	}

	texto := fmt.Sprintf("\n[%s] %s %s bytes:\n%s", quien, flecha, nombreTipoDe(body), Desglosar(body))

	if DebugActivo {
		fmt.Print(texto)
	}
	if GuardarActivo {
		logMu.Lock()
		if logArchivo != nil {
			logArchivo.WriteString(texto)
		}
		logMu.Unlock()
	}
}

func nombreTipoDe(body []byte) string {
	if len(body) < 1 {
		return "(vacío)"
	}
	return nombreTipo(body[0])
}
