// Package game implementa las reglas del PRFC-CC8-2026 v3.
//
// PARTE 2 de la implementación. Es puro: no sabe de red, ni de bytes, ni de
// goroutines. Recibe las intenciones de un ciclo (direcciones e interacciones)
// y devuelve el estado resultante. Esa pureza es lo que lo hace fácil de probar:
// se simulan partidas enteras en memoria, sin abrir un solo socket.
//
// Cómo se usa:
//
//	w := game.New(game.DefaultConfig(), nil)
//	w.AddPlayer(1, "Ana")
//	w.SpawnAll()
//	w.SetDirection(1, game.DirRight)   // Ana camina a la derecha
//	ev := w.Step(nil)                  // avanza un ciclo
//
// El servidor de la Parte 3 llamará a estos métodos; por ahora los llama el
// programa de demostración.
package game

import (
	"math"
	"sort"
)

// Direcciones (§10). Coinciden en valor con las del paquete protocol, pero se
// redefinen aquí para que el motor no dependa de la capa de red.
const (
	DirNone  = 0x00
	DirUp    = 0x01
	DirDown  = 0x02
	DirLeft  = 0x03
	DirRight = 0x04
)

// Estados de la bandera (§7).
const (
	FlagAvailable = 0x01
	FlagCarried   = 0x02
	FlagDropped   = 0x03
	FlagOutside   = 0x04
)

// Config son los parámetros de §21. DefaultConfig trae los valores recomendados.
type Config struct {
	MapSize           float64
	CircleRadius      float64
	PlayerRadius      float64
	SpawnMargin       float64
	PlayerSpeed       float64
	InteractionRadius float64
	TickIntervalMs    int

	// RequireEnteredCircle: para ganar hay que haber estado dentro del círculo
	// desde que se tomó la bandera. Evita ganar recogiendo una bandera que
	// quedó afuera. Ver el acuerdo 005 del repositorio.
	RequireEnteredCircle bool
}

func DefaultConfig() Config {
	return Config{
		MapSize:              2000,
		CircleRadius:         500,
		PlayerRadius:         15,
		SpawnMargin:          80,
		PlayerSpeed:          220,
		InteractionRadius:    60,
		TickIntervalMs:       50,
		RequireEnteredCircle: true,
	}
}

// Player es el estado de un jugador. enteredCircle es interno; no viaja.
type Player struct {
	ID        uint16
	Name      string
	X, Y      float64
	Direction uint8
	HasFlag   bool

	enteredCircle bool
}

// Flag es la bandera. Cuando está CARRIED, su posición es la del portador.
type Flag struct {
	Status  uint8
	X, Y    float64
	Carrier uint16
}

// Steal describe un cambio de portador por robo, para el mensaje FLAG_STOLEN.
type Steal struct {
	Previous uint16
	New      uint16
}

// Events son los hechos de un ciclo, que el servidor convertirá en mensajes en
// el orden de §29.11.
type Events struct {
	PickedUp []uint16 // quién recogió la bandera
	Stolen   []Steal  // robos ocurridos
	Winner   uint16   // 0 si nadie ganó este ciclo
}

// World es el estado oficial de una partida.
type World struct {
	Cfg     Config
	Tick    uint32
	Flag    Flag
	players map[uint16]*Player
	ids     []uint16 // siempre ordenado ascendente, requisito de §15
	winner  uint16
	rnd     func() float64
}

// New crea un mundo vacío con la bandera en el centro. rnd devuelve valores en
// [0,1); si es nil, se usa una secuencia determinista para que los tests sean
// reproducibles.
func New(cfg Config, rnd func() float64) *World {
	if rnd == nil {
		seed := 0.0
		rnd = func() float64 { seed += 0.618033988749; return seed - math.Floor(seed) }
	}
	return &World{
		Cfg:     cfg,
		Flag:    Flag{Status: FlagAvailable, X: 0, Y: 0},
		players: map[uint16]*Player{},
		rnd:     rnd,
	}
}

// AddPlayer registra un jugador sin colocarlo aún en el mapa.
func (w *World) AddPlayer(id uint16, name string) *Player {
	p := &Player{ID: id, Name: name}
	w.players[id] = p
	w.ids = append(w.ids, id)
	sort.Slice(w.ids, func(i, j int) bool { return w.ids[i] < w.ids[j] })
	return p
}

// RemovePlayer aplica §17: el jugador sale del mapa y, si llevaba la bandera,
// ésta cae donde estaba. Si cayó fuera del círculo, vuelve al centro para que
// nadie gane recogiéndola de una (acuerdo 005).
func (w *World) RemovePlayer(id uint16) {
	p, ok := w.players[id]
	if !ok {
		return
	}
	if w.Flag.Status == FlagCarried && w.Flag.Carrier == id {
		outside := w.distToOrigin(p.X, p.Y)-w.Cfg.PlayerRadius > w.Cfg.CircleRadius
		if outside {
			w.Flag = Flag{Status: FlagAvailable, X: 0, Y: 0}
		} else {
			w.Flag = Flag{Status: FlagDropped, X: p.X, Y: p.Y}
		}
	}
	delete(w.players, id)
	for i, v := range w.ids {
		if v == id {
			w.ids = append(w.ids[:i], w.ids[i+1:]...)
			break
		}
	}
}

// Players devuelve los jugadores en orden ascendente de ID (§15).
func (w *World) Players() []*Player {
	out := make([]*Player, 0, len(w.ids))
	for _, id := range w.ids {
		out = append(out, w.players[id])
	}
	return out
}

func (w *World) Player(id uint16) *Player { return w.players[id] }
func (w *World) Winner() uint16           { return w.winner }
func (w *World) Finished() bool           { return w.winner != 0 }

// HasFlag indica si el jugador lleva la bandera. Incluye el estado OUTSIDE
// porque el ganador la sigue cargando al cruzar el borde: el GAME_STATE final
// debe mostrarlo con la bandera, o el cliente lo dibujaría con las manos vacías.
func (w *World) HasFlag(id uint16) bool {
	return w.Flag.Carrier == id && (w.Flag.Status == FlagCarried || w.Flag.Status == FlagOutside)
}

// Spawn coloca a un jugador en un punto aleatorio fuera del círculo (§9).
func (w *World) Spawn(p *Player) {
	angle := w.rnd() * 2 * math.Pi
	d := w.Cfg.CircleRadius + w.Cfg.SpawnMargin
	half := w.Cfg.MapSize / 2
	p.X = clamp(d*math.Cos(angle), half)
	p.Y = clamp(d*math.Sin(angle), half)
	p.Direction = DirNone
}

// SpawnAll coloca a todos los jugadores registrados.
func (w *World) SpawnAll() {
	for _, p := range w.Players() {
		w.Spawn(p)
	}
}

// Reiniciar deja el mundo listo para una nueva partida sin perder a los
// jugadores conectados. Devuelve la bandera al centro, borra el ganador, pone el
// tick en cero y limpia el estado interno de cada jugador (si entró al círculo,
// si lleva la bandera). No reposiciona: eso lo hace SpawnAll al arrancar.
func (w *World) Reiniciar() {
	w.Tick = 0
	w.winner = 0
	w.Flag = Flag{Status: FlagAvailable, X: 0, Y: 0}
	for _, p := range w.players {
		p.HasFlag = false
		p.Direction = DirNone
		p.enteredCircle = false
	}
}

// SetDirection registra la dirección activa de un jugador (§10). Una dirección
// fuera de rango se ignora, que es lo más seguro.
func (w *World) SetDirection(id uint16, dir uint8) {
	if p, ok := w.players[id]; ok && dir <= DirRight {
		p.Direction = dir
	}
}

// Step ejecuta un ciclo completo en el orden de §30. interacts es el conjunto
// de jugadores que presionaron la tecla; a lo sumo se procesa uno por jugador.
func (w *World) Step(interacts map[uint16]bool) Events {
	var ev Events
	if w.Finished() {
		return ev
	}
	w.Tick++

	step := w.Cfg.PlayerSpeed * float64(w.Cfg.TickIntervalMs) / 1000.0
	half := w.Cfg.MapSize / 2

	// Pasos 3-5: mover cada jugador un paso en su dirección y recortar al mapa.
	for _, p := range w.Players() {
		switch p.Direction {
		case DirUp:
			p.Y -= step
		case DirDown:
			p.Y += step
		case DirLeft:
			p.X -= step
		case DirRight:
			p.X += step
		}
		p.X = clamp(p.X, half)
		p.Y = clamp(p.Y, half)
	}

	// Paso 6: resolver interacciones en orden ascendente de ID. La bandera
	// cambia de manos a lo sumo una vez por ciclo (§15).
	if len(interacts) > 0 {
		changed := false
		for _, p := range w.Players() {
			if changed || !interacts[p.ID] {
				continue
			}
			switch w.Flag.Status {
			case FlagAvailable, FlagDropped:
				if dist(p.X, p.Y, w.Flag.X, w.Flag.Y) <= w.Cfg.InteractionRadius {
					w.giveFlag(p)
					ev.PickedUp = append(ev.PickedUp, p.ID)
					changed = true
				}
			case FlagCarried:
				if w.Flag.Carrier == p.ID {
					continue // ya la lleva; §12: ignorar en silencio
				}
				carrier := w.players[w.Flag.Carrier]
				if carrier != nil && dist(p.X, p.Y, carrier.X, carrier.Y) <= w.Cfg.InteractionRadius {
					prev := carrier.ID
					w.giveFlag(p)
					ev.Stolen = append(ev.Stolen, Steal{Previous: prev, New: p.ID})
					changed = true
				}
			}
		}
	}

	// Paso 7: la bandera acompaña al portador; marcamos si entró al círculo.
	if w.Flag.Status == FlagCarried {
		if c := w.players[w.Flag.Carrier]; c != nil {
			w.Flag.X, w.Flag.Y = c.X, c.Y
			if w.distToOrigin(c.X, c.Y) <= w.Cfg.CircleRadius {
				c.enteredCircle = true
			}
		}
	}

	// Paso 9: condición de victoria (§16).
	if w.Flag.Status == FlagCarried {
		if c := w.players[w.Flag.Carrier]; c != nil {
			fuera := w.distToOrigin(c.X, c.Y)-w.Cfg.PlayerRadius > w.Cfg.CircleRadius
			elegible := !w.Cfg.RequireEnteredCircle || c.enteredCircle
			if fuera && elegible {
				w.winner = c.ID
				w.Flag.Status = FlagOutside
				ev.Winner = c.ID
			}
		}
	}
	return ev
}

// giveFlag entrega la bandera a p. Reinicia enteredCircle porque el acuerdo 005
// exige haber entrado al círculo desde que se tomó la bandera, no alguna vez.
func (w *World) giveFlag(p *Player) {
	w.Flag.Status = FlagCarried
	w.Flag.Carrier = p.ID
	w.Flag.X, w.Flag.Y = p.X, p.Y
	p.enteredCircle = w.distToOrigin(p.X, p.Y) <= w.Cfg.CircleRadius
}

func (w *World) distToOrigin(x, y float64) float64 { return math.Hypot(x, y) }

func dist(ax, ay, bx, by float64) float64 { return math.Hypot(ax-bx, ay-by) }

func clamp(v, limit float64) float64 {
	if v > limit {
		return limit
	}
	if v < -limit {
		return -limit
	}
	return v
}
