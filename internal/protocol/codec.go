package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

// Este archivo convierte los structs de mensajes.go en bytes y viceversa,
// siguiendo el layout del PRFC-CC8-2026 v3.
//
// Cómo leerlo: MarshalBinary escribe; UnmarshalBinary lee. Los helpers writer y
// reader implementan los tipos base de §23.1 (u8, u16, u32, i16, i32, str) para
// que el resto del código se lea casi como la tabla del documento.

var (
	ErrShort   = errors.New("mensaje binario incompleto")
	ErrType    = errors.New("tipo de mensaje desconocido")
	ErrVersion = errors.New("versión de protocolo incompatible")
)

// ---------- Helpers de escritura ----------

type writer struct{ b []byte }

func (w *writer) u8(v uint8)   { w.b = append(w.b, v) }
func (w *writer) boolean(v bool) {
	if v {
		w.u8(1)
	} else {
		w.u8(0)
	}
}
func (w *writer) u16(v uint16) { w.b = binary.BigEndian.AppendUint16(w.b, v) }
func (w *writer) u32(v uint32) { w.b = binary.BigEndian.AppendUint32(w.b, v) }
func (w *writer) i32(v int32)  { w.u32(uint32(v)) }

// str escribe un string con prefijo de longitud u8 (§23.1).
func (w *writer) str(s string) {
	bs := []byte(s)
	if len(bs) > 255 {
		bs = bs[:255]
	}
	w.u8(uint8(len(bs)))
	w.b = append(w.b, bs...)
}

// coord convierte una coordenada de mundo al i32 escalado por 100 (§24).
func coord(v float64) int32 { return int32(math.Round(v * 100)) }

func (w *writer) coord(v float64) { w.i32(coord(v)) }

func (w *writer) flag(f Flag) {
	w.u8(f.Status)
	w.u16(f.Carrier)
	w.coord(f.X)
	w.coord(f.Y)
}

// playerFull escribe un jugador con nombre (GAME_STARTED).
func (w *writer) playerFull(p Player) {
	w.u16(p.ID)
	w.str(p.Name)
	w.coord(p.X)
	w.coord(p.Y)
	w.u8(p.Direction)
	w.boolean(p.HasFlag)
}

// playerCompact escribe un jugador sin nombre (GAME_STATE, §29.6).
func (w *writer) playerCompact(p Player) {
	w.u16(p.ID)
	w.coord(p.X)
	w.coord(p.Y)
	w.u8(p.Direction)
	w.boolean(p.HasFlag)
}

// ---------- MarshalBinary ----------

// MarshalBinary serializa un mensaje al formato del PRFC. Devuelve el cuerpo sin
// el prefijo de longitud del enmarcado TCP; ese lo agrega WriteFrame.
func MarshalBinary(msg any) ([]byte, error) {
	w := &writer{}
	switch m := msg.(type) {
	case DiscoverRequest:
		w.u8(TypeDiscoverRequest)
		w.u8(Version)
	case DiscoverResponse:
		w.u8(TypeDiscoverResponse)
		w.u8(Version)
		w.u16(m.GameID)
		w.str(m.ServerName)
		w.u16(m.TCPPort)
		w.u8(m.State)
		w.u16(m.PlayerCount)
		w.u16(m.MaximumPlayers)
	case Join:
		w.u8(TypeJoin)
		w.u8(Version)
		w.str(m.Name)
	case Input:
		w.u8(TypeInput)
		w.u8(Version)
		w.u16(m.PlayerID)
		w.u8(m.Direction)
	case Interact:
		w.u8(TypeInteract)
		w.u8(Version)
		w.u16(m.PlayerID)
	case Leave:
		w.u8(TypeLeave)
		w.u8(Version)
		w.u16(m.PlayerID)
	case JoinAccepted:
		w.u8(TypeJoinAccepted)
		w.u8(Version)
		w.u16(m.PlayerID)
		w.u16(m.GameID)
	case JoinRejected:
		w.u8(TypeJoinRejected)
		w.u8(Version)
		w.u8(m.Reason)
	case LobbyState:
		w.u8(TypeLobbyState)
		w.u8(Version)
		w.u8(m.State)
		w.u8(uint8(len(m.Players)))
		for _, p := range m.Players {
			w.u16(p.ID)
			w.str(p.Name)
		}
	case GameCountdown:
		w.u8(TypeGameCountdown)
		w.u8(Version)
		w.u8(m.SecondsRemaining)
	case GameStarted:
		w.u8(TypeGameStarted)
		w.u8(Version)
		w.coord(m.MapSize)
		w.coord(m.CircleRadius)
		w.coord(m.PlayerRadius)
		w.coord(m.PlayerSpeed)
		w.coord(m.InteractionRadius)
		w.u16(m.TickIntervalMs)
		w.flag(m.Flag)
		w.u8(uint8(len(m.Players)))
		for _, p := range m.Players {
			w.playerFull(p)
		}
	case GameState:
		w.u8(TypeGameState)
		w.u8(Version)
		w.u32(m.Tick)
		w.flag(m.Flag)
		w.u8(uint8(len(m.Players)))
		for _, p := range m.Players {
			w.playerCompact(p)
		}
	case FlagPickedUp:
		w.u8(TypeFlagPickedUp)
		w.u8(Version)
		w.u32(m.Tick)
		w.u16(m.PlayerID)
	case FlagStolen:
		w.u8(TypeFlagStolen)
		w.u8(Version)
		w.u32(m.Tick)
		w.u16(m.PreviousCarrierID)
		w.u16(m.NewCarrierID)
	case PlayerDisconnected:
		w.u8(TypePlayerDisconnect)
		w.u8(Version)
		w.u16(m.PlayerID)
	case GameOver:
		w.u8(TypeGameOver)
		w.u8(Version)
		w.u16(m.WinnerID)
		w.str(m.WinnerName)
		w.u8(m.Reason)
	case Error:
		w.u8(TypeError)
		w.u8(Version)
		w.u8(m.Code)
		w.str(m.Description)
	default:
		return nil, fmt.Errorf("no sé serializar %T", msg)
	}
	return w.b, nil
}

// ---------- Helpers de lectura ----------

// reader recorre un slice de bytes. Si alguna lectura se pasa del final, marca
// err y todas las siguientes lo respetan: basta comprobar err una vez al final.
type reader struct {
	b   []byte
	i   int
	err error
}

func (r *reader) u8() uint8 {
	if r.err != nil || r.i+1 > len(r.b) {
		r.err = ErrShort
		return 0
	}
	v := r.b[r.i]
	r.i++
	return v
}
func (r *reader) boolean() bool { return r.u8() != 0 }
func (r *reader) u16() uint16 {
	if r.err != nil || r.i+2 > len(r.b) {
		r.err = ErrShort
		return 0
	}
	v := binary.BigEndian.Uint16(r.b[r.i:])
	r.i += 2
	return v
}
func (r *reader) u32() uint32 {
	if r.err != nil || r.i+4 > len(r.b) {
		r.err = ErrShort
		return 0
	}
	v := binary.BigEndian.Uint32(r.b[r.i:])
	r.i += 4
	return v
}
func (r *reader) i32() int32 { return int32(r.u32()) }
func (r *reader) coord() float64 { return float64(r.i32()) / 100 }
func (r *reader) str() string {
	n := int(r.u8())
	if r.err != nil || r.i+n > len(r.b) {
		r.err = ErrShort
		return ""
	}
	s := string(r.b[r.i : r.i+n])
	r.i += n
	return s
}

func (r *reader) flag() Flag {
	return Flag{Status: r.u8(), Carrier: r.u16(), X: r.coord(), Y: r.coord()}
}
func (r *reader) playerFull() Player {
	return Player{ID: r.u16(), Name: r.str(), X: r.coord(), Y: r.coord(),
		Direction: r.u8(), HasFlag: r.boolean()}
}
func (r *reader) playerCompact() Player {
	return Player{ID: r.u16(), X: r.coord(), Y: r.coord(),
		Direction: r.u8(), HasFlag: r.boolean()}
}

// ---------- UnmarshalBinary ----------

// UnmarshalBinary decodifica un cuerpo (sin prefijo de longitud) al struct que
// le corresponde. Valida tipo y versión antes de leer el resto (§32).
func UnmarshalBinary(body []byte) (any, error) {
	if len(body) < 2 {
		return nil, ErrShort
	}
	t := body[0]
	if body[1] != Version {
		return nil, ErrVersion
	}
	r := &reader{b: body, i: 2}

	var out any
	switch t {
	case TypeDiscoverRequest:
		out = DiscoverRequest{}
	case TypeDiscoverResponse:
		out = DiscoverResponse{GameID: r.u16(), ServerName: r.str(), TCPPort: r.u16(),
			State: r.u8(), PlayerCount: r.u16(), MaximumPlayers: r.u16()}
	case TypeJoin:
		out = Join{Name: r.str()}
	case TypeInput:
		out = Input{PlayerID: r.u16(), Direction: r.u8()}
	case TypeInteract:
		out = Interact{PlayerID: r.u16()}
	case TypeLeave:
		out = Leave{PlayerID: r.u16()}
	case TypeJoinAccepted:
		out = JoinAccepted{PlayerID: r.u16(), GameID: r.u16()}
	case TypeJoinRejected:
		out = JoinRejected{Reason: r.u8()}
	case TypeLobbyState:
		m := LobbyState{State: r.u8()}
		n := int(r.u8())
		for k := 0; k < n; k++ {
			m.Players = append(m.Players, LobbyPlayer{ID: r.u16(), Name: r.str()})
		}
		out = m
	case TypeGameCountdown:
		out = GameCountdown{SecondsRemaining: r.u8()}
	case TypeGameStarted:
		m := GameStarted{MapSize: r.coord(), CircleRadius: r.coord(), PlayerRadius: r.coord(),
			PlayerSpeed: r.coord(), InteractionRadius: r.coord(), TickIntervalMs: r.u16()}
		m.Flag = r.flag()
		n := int(r.u8())
		for k := 0; k < n; k++ {
			m.Players = append(m.Players, r.playerFull())
		}
		out = m
	case TypeGameState:
		m := GameState{Tick: r.u32()}
		m.Flag = r.flag()
		n := int(r.u8())
		for k := 0; k < n; k++ {
			m.Players = append(m.Players, r.playerCompact())
		}
		out = m
	case TypeFlagPickedUp:
		out = FlagPickedUp{Tick: r.u32(), PlayerID: r.u16()}
	case TypeFlagStolen:
		out = FlagStolen{Tick: r.u32(), PreviousCarrierID: r.u16(), NewCarrierID: r.u16()}
	case TypePlayerDisconnect:
		out = PlayerDisconnected{PlayerID: r.u16()}
	case TypeGameOver:
		out = GameOver{WinnerID: r.u16(), WinnerName: r.str(), Reason: r.u8()}
	case TypeError:
		out = Error{Code: r.u8(), Description: r.str()}
	default:
		return nil, ErrType
	}
	if r.err != nil {
		return nil, r.err
	}
	return out, nil
}

// ---------- Enmarcado TCP (§23.2) ----------

// WriteFrame escribe un cuerpo con su prefijo de longitud u16.
func WriteFrame(w io.Writer, body []byte) error {
	if len(body) > 0xFFFF {
		return fmt.Errorf("mensaje de %d bytes excede el máximo de 65535", len(body))
	}
	var hdr [2]byte
	binary.BigEndian.PutUint16(hdr[:], uint16(len(body)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(body)
	return err
}

// ReadFrame lee un mensaje enmarcado: 2 bytes de longitud y luego el cuerpo.
// Usa io.ReadFull porque TCP puede entregar la lectura partida.
func ReadFrame(r io.Reader) ([]byte, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint16(hdr[:])
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}
