// Package protocol implementa el transporte binario del PRFC-CC8-2026 v3.
//
// PARTE 1 de la implementación: solo los mensajes y su serialización a bytes.
// No hay red todavía. El objetivo de esta parte es que un INPUT se convierta
// exactamente en los bytes que dice el documento, y que todo mensaje sobreviva
// el viaje de ida (struct → bytes) y vuelta (bytes → struct).
package protocol

// Version es el byte de versión que viaja en cada mensaje (§23.4).
const Version = 3

// Códigos de tipo de mensaje (§26). Cada mensaje empieza con este byte.
const (
	TypeDiscoverRequest  = 0x01
	TypeDiscoverResponse = 0x02
	TypeJoin             = 0x10
	TypeInput            = 0x11
	TypeInteract         = 0x12
	TypeLeave            = 0x13
	TypeJoinAccepted     = 0x20
	TypeJoinRejected     = 0x21
	TypeLobbyState       = 0x22
	TypeGameCountdown    = 0x23
	TypeGameStarted      = 0x24
	TypeGameState        = 0x25
	TypeFlagPickedUp     = 0x26
	TypeFlagStolen       = 0x27
	TypePlayerDisconnect = 0x28
	TypeGameOver         = 0x29
	TypeError            = 0x2A
)

// Direcciones de movimiento (§10). Van en un solo byte.
const (
	DirNone  = 0x00
	DirUp    = 0x01
	DirDown  = 0x02
	DirLeft  = 0x03
	DirRight = 0x04
)

// Estados de la partida (§18).
const (
	StateWaiting   = 0x01
	StateStarting  = 0x02
	StateRunning   = 0x03
	StateFinished  = 0x04
	StateCancelled = 0x05
)

// Estados de la bandera (§7).
const (
	FlagAvailable = 0x01
	FlagCarried   = 0x02
	FlagDropped   = 0x03
	FlagOutside   = 0x04
)

// Motivos de JOIN_REJECTED (§25).
const (
	ReasonGameAlreadyStarted = 0x01
	ReasonGameFull           = 0x02
	ReasonInvalidName        = 0x03
	ReasonUnsupportedVersion = 0x04
)

// Códigos de ERROR (§29.12).
const (
	ErrInvalidMessage     = 0x01
	ErrInvalidEncoding    = 0x02
	ErrInvalidInput       = 0x03
	ErrUnknownPlayer      = 0x04
	ErrGameNotStarted     = 0x05
	ErrGameAlreadyStarted = 0x06
	ErrGameFinished       = 0x07
	ErrUnsupportedVersion = 0x08
)

// ---------- Structs de los mensajes ----------
//
// Cada struct refleja el layout de su sección del PRFC. Las posiciones se
// guardan como float64 en unidades de mundo; la conversión a los enteros
// escalados por 100 la hace el codec al serializar (§24).

// Player es un jugador tal como aparece en GAME_STARTED (§29.5) y GAME_STATE
// (§29.6). En GAME_STATE el Name viaja vacío: el cliente ya lo conoce.
type Player struct {
	ID        uint16
	Name      string
	X, Y      float64
	Direction uint8
	HasFlag   bool
}

// Flag es la bandera (§7). Carrier 0 significa "nadie la lleva".
type Flag struct {
	Status  uint8
	X, Y    float64
	Carrier uint16
}

type DiscoverRequest struct{}

type DiscoverResponse struct {
	GameID         uint16
	ServerName     string
	TCPPort        uint16
	State          uint8
	PlayerCount    uint16
	MaximumPlayers uint16
}

type Join struct {
	Name string
}

type Input struct {
	PlayerID  uint16
	Direction uint8
}

type Interact struct {
	PlayerID uint16
}

type Leave struct {
	PlayerID uint16
}

type JoinAccepted struct {
	PlayerID uint16
	GameID   uint16
}

type JoinRejected struct {
	Reason uint8
}

type LobbyState struct {
	State   uint8
	Players []LobbyPlayer
}

type LobbyPlayer struct {
	ID   uint16
	Name string
}

type GameCountdown struct {
	SecondsRemaining uint8
}

type GameStarted struct {
	MapSize           float64
	CircleRadius      float64
	PlayerRadius      float64
	PlayerSpeed       float64
	InteractionRadius float64
	TickIntervalMs    uint16
	Flag              Flag
	Players           []Player
}

type GameState struct {
	Tick    uint32
	Flag    Flag
	Players []Player
}

type FlagPickedUp struct {
	Tick     uint32
	PlayerID uint16
}

type FlagStolen struct {
	Tick              uint32
	PreviousCarrierID uint16
	NewCarrierID      uint16
}

type PlayerDisconnected struct {
	PlayerID uint16
}

type GameOver struct {
	WinnerID   uint16
	WinnerName string
	Reason     uint8
}

type Error struct {
	Code        uint8
	Description string
}
