package ui

import (
	"encoding/json"
	"image/color"
	"math/rand"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ctf/assets"
)

// Tema define la apariencia y la distribución de un mapa. No se escribe a mano:
// se carga desde assets/temas/<nombre>/ (tiles.json + distribucion.json). Ver
// assets/temas/README.md para el formato y cómo agregar mapas.
type Tema struct {
	Nombre string
	Pack   string // qué pack de tiles usa ("farm", "puny"); ver pack.go

	// Apariencia (de tiles.json). Los índices son tiles del pack.
	Pasto       []int      // tiles de fondo; se alternan por celda
	TierraColor color.RGBA // color del disco central (el "sembradío" o claro)
	Cultivos    []int      // sprites esparcidos dentro del círculo
	Decoracion  []int      // árboles/arbustos esparcidos afuera
	Props       []int      // objetos y animales que dan vida al campo (afuera)
	Edificio    [][]int    // matriz de tiles de un edificio (ej: granero)
	Jugador     int        // sprite base del jugador (-1 = solo disco de color)
	Bandera     int        // sprite de la bandera
	Agua        int        // tile de agua animada para los estanques (-1 = sin agua)

	// Distribución (de distribucion.json).
	CantCultivos int
	CantArboles  int
	CantProps    int
	Graneros     [][2]float64 // posiciones [x, y] de edificios (ventana 800×800)
	Estanques    [][2]float64 // centros de estanques de agua animada
	Semilla      int64        // semilla del azar para el fondo
}

// Estructuras espejo de los dos JSON de cada tema.
type tilesJSON struct {
	Pack        string  `json:"pack"`
	Pasto       []int   `json:"pasto"`
	TierraColor string  `json:"tierraColor"`
	Cultivos    []int   `json:"cultivos"`
	Decoracion  []int   `json:"decoracion"`
	Props       []int   `json:"props"`
	Edificio    [][]int `json:"edificio"`
	Jugador     *int    `json:"jugador"` // puntero: ausente → -1 (solo disco)
	Bandera     int     `json:"bandera"`
	Agua        *int    `json:"agua"` // puntero: ausente → -1 (sin agua)
}

type distribucionJSON struct {
	CantCultivos int          `json:"cantCultivos"`
	CantArboles  int          `json:"cantArboles"`
	CantProps    int          `json:"cantProps"`
	Graneros     [][2]float64 `json:"graneros"`
	Estanques    [][2]float64 `json:"estanques"`
	Semilla      int64        `json:"semilla"`
}

var (
	temas    []Tema
	temasUna sync.Once
	rngTema  = rand.New(rand.NewSource(time.Now().UnixNano()))
)

// cargarTemas descubre y parsea todos los temas de assets/temas/ una sola vez.
// Si algo falla o no hay ninguno, cae en un tema por defecto para que el juego
// siga funcionando.
func cargarTemas() {
	temasUna.Do(func() {
		entradas, err := assets.Temas.ReadDir("temas")
		if err != nil {
			temas = []Tema{temaPorDefecto()}
			return
		}
		var nombres []string
		for _, e := range entradas {
			if e.IsDir() {
				nombres = append(nombres, e.Name())
			}
		}
		sort.Strings(nombres) // orden estable
		for _, n := range nombres {
			t, err := leerTema(n)
			if err != nil {
				continue // saltamos un tema mal formado sin romper el resto
			}
			temas = append(temas, t)
		}
		if len(temas) == 0 {
			temas = []Tema{temaPorDefecto()}
		}
	})
}

// leerTema arma un Tema a partir de sus dos JSON.
func leerTema(nombre string) (Tema, error) {
	dir := path.Join("temas", nombre)
	var tj tilesJSON
	if err := leerJSON(path.Join(dir, "tiles.json"), &tj); err != nil {
		return Tema{}, err
	}
	var dj distribucionJSON
	if err := leerJSON(path.Join(dir, "distribucion.json"), &dj); err != nil {
		return Tema{}, err
	}
	return Tema{
		Nombre:       capitalizar(nombre),
		Pack:         tj.Pack,
		Pasto:        tj.Pasto,
		TierraColor:  hexAColor(tj.TierraColor),
		Cultivos:     tj.Cultivos,
		Decoracion:   tj.Decoracion,
		Props:        tj.Props,
		Edificio:     tj.Edificio,
		Jugador:      opcional(tj.Jugador, -1),
		Bandera:      tj.Bandera,
		Agua:         opcional(tj.Agua, -1),
		CantCultivos: dj.CantCultivos,
		CantArboles:  dj.CantArboles,
		CantProps:    dj.CantProps,
		Graneros:     dj.Graneros,
		Estanques:    dj.Estanques,
		Semilla:      dj.Semilla,
	}, nil
}

// opcional devuelve *p si está presente, o def si es nil.
func opcional(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

func leerJSON(nombre string, v any) error {
	datos, err := assets.Temas.ReadFile(nombre)
	if err != nil {
		return err
	}
	return json.Unmarshal(datos, v)
}

// hexAColor convierte "#rrggbb" (o "#rrggbbaa") a color.RGBA. Ante un valor
// inválido devuelve un marrón tierra por defecto.
func hexAColor(s string) color.RGBA {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) < 6 {
		return color.RGBA{198, 132, 90, 255}
	}
	r, _ := strconv.ParseUint(s[0:2], 16, 8)
	g, _ := strconv.ParseUint(s[2:4], 16, 8)
	b, _ := strconv.ParseUint(s[4:6], 16, 8)
	a := uint64(255)
	if len(s) >= 8 {
		a, _ = strconv.ParseUint(s[6:8], 16, 8)
	}
	return color.RGBA{uint8(r), uint8(g), uint8(b), uint8(a)}
}

func capitalizar(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// temaForzado, si no está vacío, fija el tema en vez de elegir al azar (flag -tema).
var temaForzado string

// ForzarTema fija el tema por nombre (case-insensitive). Vacío = al azar.
func ForzarTema(nombre string) { temaForzado = nombre }

// temaAleatorio elige un tema del registro: el forzado si se indicó uno válido,
// o uno al azar.
func temaAleatorio() Tema {
	cargarTemas()
	if temaForzado != "" {
		for i := range temas {
			if strings.EqualFold(temas[i].Nombre, temaForzado) {
				return temas[i]
			}
		}
	}
	return temas[rngTema.Intn(len(temas))]
}

// temaPorDefecto es el respaldo si no se pueden leer las carpetas de temas.
// Refleja el tema Campo (pack Tiny Farm).
func temaPorDefecto() Tema {
	return Tema{
		Nombre:       "Campo",
		Pack:         "farm",
		Pasto:        []int{105, 117},
		TierraColor:  color.RGBA{198, 132, 90, 255},
		Cultivos:     []int{8, 20, 32, 44, 54},
		Decoracion:   []int{27, 15, 39},
		Props:        []int{85, 96, 124, 74, 76, 120, 121, 122},
		Edificio:     [][]int{{90, 91, 92}, {102, 103, 104}, {114, 115, 116}},
		Jugador:      108,
		Bandera:      83,
		Agua:         -1,
		CantCultivos: 7,
		CantArboles:  48,
		CantProps:    20,
		Graneros:     [][2]float64{{110, 120}, {690, 130}, {130, 690}, {680, 680}},
		Semilla:      1,
	}
}
