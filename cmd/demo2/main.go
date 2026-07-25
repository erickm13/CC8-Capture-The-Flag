// Demo de la PARTE 2: el motor de reglas.
//
// Simula una partida completa entre dos jugadores y muestra qué pasa en cada
// ciclo: posiciones, quién lleva la bandera, robos y victoria. Dibuja el mapa
// en ASCII para que puedas seguir el movimiento sin interfaz gráfica.
//
// Correlo así:
//
//	go run ./cmd/demo2
//
// No hay red: es todo el motor de reglas corriendo en memoria. Lo que ves acá
// es exactamente lo que el servidor de la Parte 3 va a difundir por la red.
package main

import (
	"fmt"
	"math"
	"strings"

	"ctf/internal/game"
)

func main() {
	fmt.Println("=== PARTE 2: motor de reglas ===")
	fmt.Println()

	// Mundo chico y lento para que la partida quepa en pocos ciclos y se pueda
	// seguir a mano. Todos los valores son válidos; solo cambian de los de §21.
	cfg := game.DefaultConfig()
	cfg.MapSize = 800
	cfg.CircleRadius = 200
	cfg.PlayerSpeed = 400
	cfg.TickIntervalMs = 100 // 400 * 0.1 = 40 unidades por ciclo

	w := game.New(cfg, nil)
	ana := w.AddPlayer(1, "Ana")
	beto := w.AddPlayer(2, "Beto")
	// Posiciones fijas en vez de aleatorias, para que la demo sea reproducible.
	// Beto arranca más cerca del centro para que la persecución sea reñida.
	ana.X, ana.Y = -280, 0
	beto.X, beto.Y = 240, 0

	fmt.Printf("Mundo: mapa %.0f, círculo radio %.0f, %d unidades por ciclo\n",
		cfg.MapSize, cfg.CircleRadius, int(cfg.PlayerSpeed*float64(cfg.TickIntervalMs)/1000))
	fmt.Println("Ana entra por la izquierda, Beto por la derecha. Bandera en el centro.")
	fmt.Println()

	// Guion de la partida: qué hace cada jugador en cada ciclo.
	// dir cambia la dirección; act presiona la tecla de interacción.
	type accion struct {
		tick int
		id   uint16
		dir  uint8
		act  bool
	}
	// Guion de la partida. Nota sobre el orden: dentro de un ciclo el movimiento
	// se procesa ANTES que la interacción (§30), así que un jugador toma la
	// bandera desde donde quedó tras moverse ese ciclo.
	//
	// Ana llega primero al centro y toma la bandera. Beto, que venía desde la
	// derecha, la intercepta cuando Ana empieza a huir y se la roba. Ana no
	// logra recuperarla y Beto sale por la derecha.
	guion := []accion{
		{tick: 1, id: 1, dir: game.DirRight}, // Ana va hacia el centro (derecha)
		{tick: 1, id: 2, dir: game.DirLeft},  // Beto va hacia el centro (izquierda)
		{tick: 6, id: 1, act: true},          // Ana toma la bandera al llegar
		{tick: 7, id: 1, dir: game.DirLeft},  // Ana intenta huir a la izquierda
		{tick: 7, id: 2, act: true},          // Beto, ya cerca, se la roba
		{tick: 8, id: 2, dir: game.DirRight}, // Beto huye a la derecha con la bandera
		{tick: 8, id: 1, dir: game.DirRight}, // Ana lo persigue, pero desde atrás
	}

	for tick := 1; tick <= 30 && !w.Finished(); tick++ {
		// Aplicar las acciones de este ciclo.
		interacts := map[uint16]bool{}
		for _, a := range guion {
			if a.tick != tick {
				continue
			}
			if a.act {
				interacts[a.id] = true
			} else {
				w.SetDirection(a.id, a.dir)
			}
		}

		ev := w.Step(interacts)

		// Reportar los eventos del ciclo.
		for _, id := range ev.PickedUp {
			fmt.Printf(">>> tick %d: %s TOMÓ la bandera\n", tick, nombre(w, id))
		}
		for _, s := range ev.Stolen {
			fmt.Printf(">>> tick %d: %s le ROBÓ la bandera a %s\n",
				tick, nombre(w, s.New), nombre(w, s.Previous))
		}

		// Dibujar el estado cada pocos ciclos, y siempre en los interesantes.
		interesante := len(ev.PickedUp) > 0 || len(ev.Stolen) > 0 || w.Finished()
		if tick <= 2 || interesante {
			dibujar(w, tick)
		}

		if w.Finished() {
			fmt.Printf("\n🏁 GANÓ %s en el tick %d — salió del círculo con la bandera\n",
				nombre(w, w.Winner()), tick)
		}
	}

	if !w.Finished() {
		fmt.Println("\n(la partida no terminó en 30 ciclos)")
	}
}

func nombre(w *game.World, id uint16) string {
	if p := w.Player(id); p != nil {
		return p.Name
	}
	return fmt.Sprintf("P%02d", id)
}

// dibujar muestra el mapa en ASCII con el círculo, la bandera y los jugadores.
func dibujar(w *game.World, tick int) {
	const cols, rows = 41, 17
	half := w.Cfg.MapSize / 2

	grid := make([][]rune, rows)
	for i := range grid {
		grid[i] = make([]rune, cols)
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}
	col := func(x float64) int { return int(math.Round((x + half) / (2 * half) * float64(cols-1))) }
	row := func(y float64) int { return int(math.Round((y + half) / (2 * half) * float64(rows-1))) }
	put := func(x, y float64, ch rune) {
		c, r := col(x), row(y)
		if r >= 0 && r < rows && c >= 0 && c < cols {
			grid[r][c] = ch
		}
	}

	// El borde del círculo.
	for a := 0.0; a < 2*math.Pi; a += 0.03 {
		put(w.Cfg.CircleRadius*math.Cos(a), w.Cfg.CircleRadius*math.Sin(a), '.')
	}
	// La bandera, si está en el suelo.
	if w.Flag.Status != game.FlagCarried {
		put(w.Flag.X, w.Flag.Y, 'F')
	}
	// Los jugadores: minúscula si no llevan bandera, mayúscula si la llevan.
	for _, p := range w.Players() {
		ch := rune('a' - 1 + int(p.ID))
		if w.HasFlag(p.ID) {
			ch = rune('A' - 1 + int(p.ID))
		}
		put(p.X, p.Y, ch)
	}

	fmt.Printf("tick %2d  ", tick)
	estado := "bandera libre"
	switch w.Flag.Status {
	case game.FlagCarried:
		estado = "la lleva " + nombre(w, w.Flag.Carrier)
	case game.FlagOutside:
		estado = "¡fuera del círculo!"
	}
	fmt.Printf("(%s)\n", estado)
	for _, p := range w.Players() {
		fmt.Printf("         %s en (%.0f, %.0f)  dist al centro: %.0f\n",
			p.Name, p.X, p.Y, math.Hypot(p.X, p.Y))
	}
	fmt.Println("  +" + strings.Repeat("-", cols) + "+")
	for _, r := range grid {
		fmt.Println("  |" + string(r) + "|")
	}
	fmt.Println("  +" + strings.Repeat("-", cols) + "+")
	fmt.Println()
}
