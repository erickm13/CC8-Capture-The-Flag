// Package ui es la interfaz gráfica con Ebitengine (PARTE 6).
//
// Su única responsabilidad es dibujar y leer el teclado. Toda la lógica del
// juego y la red están en internal/client y en el servidor: la interfaz solo
// llama a client.Snapshot() para saber qué dibujar, y a client.Mover() /
// client.Interactuar() cuando el jugador presiona una tecla.
//
// Esa separación es la que hace que la parte visual sea sencilla: no hay estado
// de juego acá, solo pixeles.
package ui

import (
	"fmt"
	"image/color"
	"math"
	"math/rand"
	"time"

	"ctf/internal/client"
	"ctf/internal/discovery"
	"ctf/internal/protocol"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Tamaño de la ventana en píxeles.
const (
	Ancho = 800
	Alto  = 800
)

// modo indica qué pantalla muestra la ventana en cada momento.
type modo int

const (
	modoSeleccion modo = iota // eligiendo servidor de la lista
	modoJuego                 // conectado y jugando
	modoError                 // no se pudo conectar; muestra el mensaje
)

// Juego implementa la interfaz ebiten.Game: Update (lógica por cuadro), Draw
// (dibujo) y Layout (tamaño). Ebitengine llama Update y Draw ~60 veces por
// segundo.
//
// Una sola ventana maneja las dos pantallas (selección y juego) cambiando de
// modo. Es importante NO cerrar y reabrir la ventana: Ebitengine no permite
// llamar a RunGame dos veces en el mismo proceso.
type Juego struct {
	nombre string

	// Apariencia (sprites pixel-art).
	tema       Tema
	fondo      *ebiten.Image // fondo del juego (pasto + sembradío); se cachea
	fondoMap   float64       // MapSize con el que se construyó el fondo
	fondoLobby *ebiten.Image // fondo del lobby (pasto tileado); se cachea

	// Capa de agua animada: celdas (esquina sup-izq, en píxeles) que se
	// redibujan cada cuadro con el frame actual del tile de agua.
	aguaCeldas [][2]float64

	// Estado del juego (modo jugando).
	cli                *client.Cliente
	teclaInteractAntes bool

	// Estado de la selección (modo seleccionando).
	m          modo
	esc        *discovery.Escáner
	dport      int
	filas      []discovery.Servidor
	mouseAntes bool
	errMsg     string
}

// Nuevo crea la interfaz con un cliente ya conectado (arranca jugando). Se usa
// cuando el servidor se eligió por terminal o con -addr.
func Nuevo(cli *client.Cliente, nombre string) *Juego {
	return &Juego{cli: cli, nombre: nombre, m: modoJuego, tema: temaAleatorio()}
}

// NuevoConSeleccion crea la interfaz arrancando en la pantalla de selección de
// servidores. La conexión se hace adentro, al elegir, así la ventana nunca se
// cierra ni se reabre.
func NuevoConSeleccion(nombre string, dport int) *Juego {
	esc := discovery.NuevoEscáner(dport, 1200*time.Millisecond, 2*time.Second)
	esc.Iniciar()
	return &Juego{nombre: nombre, m: modoSeleccion, esc: esc, dport: dport, tema: temaAleatorio()}
}

// Update se llama una vez por cuadro. Aquí leemos el teclado y se lo mandamos al
// servidor a través del cliente. No calculamos posiciones: eso lo hace el
// servidor y nos llega en el Snapshot.
func (g *Juego) Update() error {
	// En modo selección, la ventana muestra la lista y espera un clic.
	if g.m == modoSeleccion {
		return g.updateSeleccion()
	}
	if g.m == modoError {
		if ebiten.IsKeyPressed(ebiten.KeyEscape) {
			return errCerrar
		}
		return nil
	}

	s := g.cli.Snapshot()

	// Si la partida terminó o nos desconectamos, cerrar la ventana.
	if s.Estado == protocol.StateFinished || !s.Conectado {
		// Damos un momento para que se vea el cartel de fin; en un juego real
		// se mostraría una pantalla de resultado. Por simplicidad, seguimos
		// dibujando y no cerramos automáticamente.
	}

	if s.Estado != protocol.StateRunning {
		return nil // en el lobby o la cuenta regresiva no se mueve
	}

	// Leer las flechas (o WASD) y traducirlas a una dirección del protocolo.
	// Si no hay ninguna tecla, la dirección es NONE (quieto).
	dir := uint8(protocol.DirNone)
	switch {
	case ebiten.IsKeyPressed(ebiten.KeyArrowUp) || ebiten.IsKeyPressed(ebiten.KeyW):
		dir = protocol.DirUp
	case ebiten.IsKeyPressed(ebiten.KeyArrowDown) || ebiten.IsKeyPressed(ebiten.KeyS):
		dir = protocol.DirDown
	case ebiten.IsKeyPressed(ebiten.KeyArrowLeft) || ebiten.IsKeyPressed(ebiten.KeyA):
		dir = protocol.DirLeft
	case ebiten.IsKeyPressed(ebiten.KeyArrowRight) || ebiten.IsKeyPressed(ebiten.KeyD):
		dir = protocol.DirRight
	}
	// Mandamos la dirección en cada cuadro; el cliente/servidor la mantienen
	// vigente. Para ahorrar mensajes, un juego más pulido solo la enviaría al
	// cambiar, pero así es más simple y correcto.
	g.cli.Mover(dir)

	// Tecla de interacción: Espacio o Enter. Solo al presionarla (no mientras
	// se mantiene), para no inundar al servidor.
	interact := ebiten.IsKeyPressed(ebiten.KeySpace) || ebiten.IsKeyPressed(ebiten.KeyEnter)
	if interact && !g.teclaInteractAntes {
		g.cli.Interactuar()
	}
	g.teclaInteractAntes = interact

	return nil
}

// Draw dibuja todo el cuadro. Se llama después de Update.
func (g *Juego) Draw(pantalla *ebiten.Image) {
	pantalla.Fill(color.RGBA{18, 18, 24, 255}) // fondo oscuro

	// Despachar según la pantalla actual.
	if g.m == modoSeleccion {
		g.drawSeleccion(pantalla)
		return
	}
	if g.m == modoError {
		_ = cargarAssets(g.tema.Pack) // carga tiles+fuente (ignora error: hay respaldo)
		escribir(pantalla, "No se pudo conectar al servidor:", 40, 60, tamMedio, color.RGBA{240, 120, 100, 255}, false)
		escribir(pantalla, g.errMsg, 40, 96, tamChico, color.White, false)
		escribir(pantalla, "Escape para salir.", 40, 140, tamChico, color.RGBA{200, 200, 210, 255}, false)
		return
	}

	s := g.cli.Snapshot()

	// Antes de que empiece la partida, mostramos el estado del lobby.
	if s.Estado != protocol.StateRunning && s.Estado != protocol.StateFinished {
		g.dibujarLobby(pantalla, s)
		return
	}

	cfg := s.Config
	if cfg.MapSize == 0 {
		return // todavía no llegó GAME_STARTED
	}
	if cargarAssets(g.tema.Pack) != nil {
		return // sin sprites no hay nada que dibujar (raro: están embebidos)
	}

	// Conversión de unidades de mundo a píxeles. El mundo va de -MapSize/2 a
	// +MapSize/2; la pantalla de 0 a Ancho. El eje y de la pantalla ya crece
	// hacia abajo, igual que el del protocolo (§5), así que no hay que invertir.
	escala := float64(Ancho) / cfg.MapSize
	aPantalla := func(x, y float64) (float32, float32) {
		px := (x + cfg.MapSize/2) * escala
		py := (y + cfg.MapSize/2) * escala
		return float32(px), float32(py)
	}

	// Fondo pre-renderizado (pasto + disco de sembradío + decoración). Se
	// construye una sola vez por tamaño de mapa y se reutiliza en cada cuadro.
	if g.fondo == nil || g.fondoMap != cfg.MapSize {
		g.fondo = g.construirFondo(cfg)
		g.fondoMap = cfg.MapSize
	}
	pantalla.DrawImage(g.fondo, nil)

	// Capa de agua animada (estanques): se redibuja cada cuadro con el frame
	// actual. El fondo estático deja huecos de pasto donde va el agua.
	if g.tema.Agua >= 0 && len(g.aguaCeldas) > 0 {
		ms := time.Now().UnixMilli()
		img := tileAnimado(g.tema.Agua, ms)
		for _, c := range g.aguaCeldas {
			dibujarTile(pantalla, img, c[0], c[1], 2)
		}
	}

	// Cuadrado rojo de interacción: si estoy a tiro de tomar/robar la bandera,
	// resalto el objetivo (como el indicador del mockup de referencia).
	g.dibujarIndicadorInteract(pantalla, s, cfg, escala, aPantalla)

	// La bandera, si está en el suelo (girasol).
	if s.Bandera.Status == protocol.FlagAvailable || s.Bandera.Status == protocol.FlagDropped {
		fx, fy := aPantalla(s.Bandera.X, s.Bandera.Y)
		dibujarTileCentrado(pantalla, tile(g.tema.Bandera), float64(fx), float64(fy), 2)
	}

	// Los jugadores: un disco de color (equipo) como base y el sprite encima.
	for _, p := range s.Jugadores {
		px, py := aPantalla(p.X, p.Y)
		col := colorJugador(p.ID, p.ID == s.MiID)
		radio := float32(cfg.PlayerRadius * escala)
		if radio < 12 {
			radio = 12
		}
		vector.DrawFilledCircle(pantalla, px, py, radio, col, true)
		if p.ID == s.MiID {
			// Aro blanco para reconocerme de un vistazo.
			vector.StrokeCircle(pantalla, px, py, radio+2, 2, color.RGBA{255, 255, 255, 255}, true)
		}
		// Sprite del jugador encima del disco. Si el tema no tiene sprite de
		// jugador (Jugador < 0), queda solo el disco de color.
		if g.tema.Jugador >= 0 {
			factor := float64(radio*2.4) / 16
			dibujarTileCentrado(pantalla, tile(g.tema.Jugador), float64(px), float64(py), factor)
		}

		// Si lleva la bandera, un aro amarillo y un girasol chico encima.
		if p.HasFlag {
			vector.StrokeCircle(pantalla, px, py, radio+6, 2, color.RGBA{240, 200, 40, 255}, true)
			dibujarTileCentrado(pantalla, tile(g.tema.Bandera), float64(px), float64(py)-float64(radio)-10, 1)
		}

		// El nombre encima. En GAME_STATE no viene el nombre (§29.6); Nombre()
		// lo busca por ID (recibido en GAME_STARTED/LOBBY_STATE) y cae en "P01".
		etiqueta := s.Nombre(p.ID)
		escribir(pantalla, etiqueta, float64(px), float64(py)-float64(radio)-20, tamChico, color.White, true)
	}

	// Panel de información arriba.
	g.dibujarHUD(pantalla, s)
}

// dibujarIndicadorInteract dibuja un recuadro rojo alrededor de la bandera (o de
// quien la lleva) cuando el jugador propio está a distancia de interactuar.
func (g *Juego) dibujarIndicadorInteract(pantalla *ebiten.Image, s client.Snapshot, cfg protocol.GameStarted, escala float64, aPantalla func(x, y float64) (float32, float32)) {
	yo := s.Yo()
	if yo == nil {
		return
	}
	var ox, oy float64
	switch s.Bandera.Status {
	case protocol.FlagCarried:
		p := s.Portador()
		if p == nil || p.ID == s.MiID {
			return // no me marco a mí mismo
		}
		ox, oy = p.X, p.Y
	case protocol.FlagAvailable, protocol.FlagDropped:
		ox, oy = s.Bandera.X, s.Bandera.Y
	default:
		return
	}
	if math.Hypot(ox-yo.X, oy-yo.Y) > cfg.InteractionRadius {
		return
	}
	sx, sy := aPantalla(ox, oy)
	lado := float32(cfg.PlayerRadius*escala) * 2.4
	if lado < 26 {
		lado = 26
	}
	vector.StrokeRect(pantalla, sx-lado/2, sy-lado/2, lado, lado, 2, color.RGBA{240, 60, 60, 255}, true)
}

// construirFondo arma la imagen de fondo del mapa una sola vez: pasto en toda la
// grilla, el disco central texturizado como sembradío (recortado al círculo real
// del juego), cultivos adentro y decoración afuera. Usa una semilla fija para
// que el fondo sea estable entre cuadros.
func (g *Juego) construirFondo(cfg protocol.GameStarted) *ebiten.Image {
	img := ebiten.NewImage(Ancho, Alto)
	rng := rand.New(rand.NewSource(g.tema.Semilla))
	g.aguaCeldas = nil // se recalcula abajo si el tema tiene estanques

	// Pasto: grilla de tiles escalados, variando el tile por celda.
	const celda = 32                 // tamaño en pantalla de cada tile de pasto
	const factorPasto = celda / 16.0 // 16px → 32px
	for y := 0; y < Alto; y += celda {
		for x := 0; x < Ancho; x += celda {
			t := g.tema.Pasto[rng.Intn(len(g.tema.Pasto))]
			dibujarTile(img, tile(t), float64(x), float64(y), factorPasto)
		}
	}

	// El disco de tierra (sembradío). El mundo (0,0) cae en el centro de la
	// pantalla independientemente de MapSize.
	escala := float64(Ancho) / cfg.MapSize
	cx, cy := float32(Ancho)/2, float32(Alto)/2
	r := float32(cfg.CircleRadius * escala)
	vector.DrawFilledCircle(img, cx, cy, r, g.tema.TierraColor, true)

	// Pocos cultivos dentro del círculo, y lejos del centro para no competir con
	// la bandera: solo unos brotes cerca del borde que insinúan el sembradío.
	for i := 0; i < g.tema.CantCultivos; i++ {
		ang := rng.Float64() * 2 * math.Pi
		rad := (0.55 + 0.4*rng.Float64()) * (float64(r) - 24) // anillo exterior
		px := float64(cx) + rad*math.Cos(ang)
		py := float64(cy) + rad*math.Sin(ang)
		c := g.tema.Cultivos[rng.Intn(len(g.tema.Cultivos))]
		dibujarTileCentrado(img, tile(c), px, py, 1.5)
	}

	// Edificios (graneros) en posiciones fijas fuera del círculo, para que el
	// pasto no se vea vacío. Guardamos sus zonas para no encimar decoración.
	type zona struct{ x, y, r float64 }
	var ocupado []zona
	if len(g.tema.Edificio) > 0 {
		const factorEd = 3.0
		alto := float64(len(g.tema.Edificio)) * 16 * factorEd
		for _, sp := range g.tema.Graneros {
			g.dibujarEdificio(img, g.tema.Edificio, sp[0], sp[1], factorEd)
			ocupado = append(ocupado, zona{sp[0], sp[1], alto * 0.6})
		}
	}

	libre := func(x, y, margen float64) bool {
		if math.Hypot(x-float64(cx), y-float64(cy)) < float64(r)+margen {
			return false
		}
		for _, z := range ocupado {
			if math.Hypot(x-z.x, y-z.y) < z.r+margen {
				return false
			}
		}
		return true
	}

	// Árboles y arbustos, más densos, salpicando todo el pasto.
	for i := 0; i < g.tema.CantArboles; i++ {
		x := rng.Float64() * Ancho
		y := rng.Float64() * Alto
		if !libre(x, y, 24) {
			continue
		}
		d := g.tema.Decoracion[rng.Intn(len(g.tema.Decoracion))]
		dibujarTileCentrado(img, tile(d), x, y, 1.6)
	}

	// Props y animales (heno, barriles, vacas, ovejas...) para darle vida.
	for i := 0; i < g.tema.CantProps; i++ {
		x := rng.Float64() * Ancho
		y := rng.Float64() * Alto
		if !libre(x, y, 22) {
			continue
		}
		p := g.tema.Props[rng.Intn(len(g.tema.Props))]
		dibujarTileCentrado(img, tile(p), x, y, 1.5)
	}

	// Estanques de agua animada: NO se hornean (deben animarse). Guardamos las
	// celdas dentro de cada estanque para redibujarlas cada cuadro en Draw.
	if g.tema.Agua >= 0 {
		const pondR = 84.0
		for _, e := range g.tema.Estanques {
			for y := e[1] - pondR; y <= e[1]+pondR; y += celda {
				for x := e[0] - pondR; x <= e[0]+pondR; x += celda {
					// centro de la celda dentro del radio del estanque
					if math.Hypot(x+celda/2-e[0], y+celda/2-e[1]) <= pondR {
						g.aguaCeldas = append(g.aguaCeldas, [2]float64{x, y})
					}
				}
			}
		}
	}

	// Contorno tenue del círculo, para que el borde del área se lea claro.
	vector.StrokeCircle(img, cx, cy, r, 2, color.RGBA{120, 90, 60, 255}, true)
	return img
}

// dibujarEdificio dibuja un edificio multi-tile (matriz de índices) centrado en
// (cx, cy) y escalado por factor. Cada celda es un tile de 16×16.
func (g *Juego) dibujarEdificio(dst *ebiten.Image, ed [][]int, cx, cy, factor float64) {
	celda := 16 * factor
	ancho := float64(len(ed[0])) * celda
	alto := float64(len(ed)) * celda
	x0 := cx - ancho/2
	y0 := cy - alto/2
	for fila, cols := range ed {
		for col, idx := range cols {
			dibujarTile(dst, tile(idx), x0+float64(col)*celda, y0+float64(fila)*celda, factor)
		}
	}
}

func (g *Juego) dibujarLobby(pantalla *ebiten.Image, s client.Snapshot) {
	// Sin sprites caemos al lobby de texto plano (no debería pasar: van embebidos).
	if cargarAssets(g.tema.Pack) != nil {
		g.dibujarLobbyTexto(pantalla, s)
		return
	}

	// Fondo: pasto tileado (cacheado, determinista para que no titile).
	if g.fondoLobby == nil {
		g.fondoLobby = g.construirFondoLobby()
	}
	pantalla.DrawImage(g.fondoLobby, nil)

	// Panel de madera centrado, tamaño según cantidad de jugadores.
	const px, pw = 180, 440
	filas := len(s.Lobby)
	ph := 200 + float64(filas)*30
	if ph > 560 {
		ph = 560
	}
	py := (float64(Alto) - ph) / 2
	dibujarPanel(pantalla, panelClaro, px, py, pw, ph)

	// Barra de título (panel oscuro) arriba del panel.
	dibujarPanel(pantalla, panelOscuro, px+24, py+20, pw-48, 54)
	titulo := "SALA DE ESPERA"
	tituloCol := color.RGBA{255, 220, 150, 255}
	if s.Estado == protocol.StateStarting {
		titulo = fmt.Sprintf("¡EMPIEZA EN %d!", s.Countdown)
		tituloCol = color.RGBA{140, 240, 150, 255}
	}
	escribir(pantalla, titulo, px+pw/2, py+30, tamMedio, tituloCol, true)

	// Lista de jugadores, con un punto de color de equipo por cada uno.
	yFila := py + 96
	escribir(pantalla, "Jugadores:", px+34, yFila-6, tamChico, color.RGBA{230, 220, 200, 255}, false)
	yFila += 22
	for _, p := range s.Lobby {
		col := colorJugador(p.ID, p.ID == s.MiID)
		vector.DrawFilledCircle(pantalla, float32(px)+44, float32(yFila)+8, 7, col, true)
		linea := fmt.Sprintf("P%02d  %s", p.ID, p.Name)
		if p.ID == s.MiID {
			linea += "  (yo)"
		}
		escribir(pantalla, linea, px+60, yFila, tamChico, color.White, false)
		yFila += 30
	}

	escribir(pantalla, "Esperando al anfitrión...", px+pw/2, py+ph-32, tamChico, color.RGBA{210, 200, 185, 255}, true)
}

// dibujarLobbyTexto es el lobby de respaldo, sin sprites (solo texto).
func (g *Juego) dibujarLobbyTexto(pantalla *ebiten.Image, s client.Snapshot) {
	y := 40
	titulo := "SALA DE ESPERA"
	if s.Estado == protocol.StateStarting {
		titulo = fmt.Sprintf("¡EMPIEZA EN %d!", s.Countdown)
	}
	ebitenutil.DebugPrintAt(pantalla, titulo, Ancho/2-60, y)
	y += 40
	ebitenutil.DebugPrintAt(pantalla, "Jugadores conectados:", 40, y)
	y += 24
	for _, p := range s.Lobby {
		linea := fmt.Sprintf("  P%02d  %s", p.ID, p.Name)
		if p.ID == s.MiID {
			linea += "  (yo)"
		}
		ebitenutil.DebugPrintAt(pantalla, linea, 40, y)
		y += 20
	}
	ebitenutil.DebugPrintAt(pantalla, "Esperando a que el anfitrión inicie la partida...", 40, Alto-40)
}

// construirFondoLobby arma un fondo de pasto tileado, estable entre cuadros
// (elección de tile determinista por celda, sin azar que titile).
func (g *Juego) construirFondoLobby() *ebiten.Image {
	img := ebiten.NewImage(Ancho, Alto)
	const celda = 32
	const factor = celda / 16.0
	cols := len(g.tema.Pasto)
	for fy := 0; fy < Alto; fy += celda {
		for fx := 0; fx < Ancho; fx += celda {
			t := g.tema.Pasto[((fx/celda)+(fy/celda))%cols]
			dibujarTile(img, tile(t), float64(fx), float64(fy), factor)
		}
	}
	// Velo oscuro para que el panel y el texto resalten.
	vector.DrawFilledRect(img, 0, 0, Ancho, Alto, color.RGBA{0, 0, 0, 90}, true)
	return img
}

func (g *Juego) dibujarHUD(pantalla *ebiten.Image, s client.Snapshot) {
	estado := "bandera libre"
	switch s.Bandera.Status {
	case protocol.FlagCarried:
		estado = fmt.Sprintf("la lleva %s", s.Nombre(s.Bandera.Carrier))
	case protocol.FlagDropped:
		estado = "bandera caída"
	}

	// Franja superior semitransparente para que el texto se lea sobre el mapa.
	vector.DrawFilledRect(pantalla, 0, 0, Ancho, 30, color.RGBA{0, 0, 0, 130}, true)
	escribir(pantalla, fmt.Sprintf("tick %d", s.Tick), 8, 6, tamChico, color.RGBA{200, 210, 230, 255}, false)
	escribir(pantalla, estado, 130, 6, tamChico, color.RGBA{255, 225, 150, 255}, false)
	escribirDer(pantalla, "WASD/flechas: mover   espacio: tomar/robar", Ancho-8, 6, tamChico, color.RGBA{190, 200, 210, 255})

	if s.Estado == protocol.StateFinished {
		msg := fmt.Sprintf("GANÓ %s", s.Ganador)
		col := color.RGBA{255, 230, 140, 255}
		if s.GanadorID == s.MiID {
			msg = "¡GANASTE!"
			col = color.RGBA{140, 240, 150, 255}
		}
		// Cartel central sobre un panel oscuro.
		vector.DrawFilledRect(pantalla, 0, Alto/2-44, Ancho, 88, color.RGBA{0, 0, 0, 150}, true)
		escribir(pantalla, msg, Ancho/2, Alto/2-18, tamGrande, col, true)
	}
}

// colorJugador da un color estable por ID. El jugador propio siempre es verde
// brillante para distinguirlo.
func colorJugador(id uint16, soyYo bool) color.Color {
	if soyYo {
		return color.RGBA{80, 240, 120, 255}
	}
	// Colores derivados del ID para que cada jugador tenga el suyo.
	colores := []color.RGBA{
		{240, 100, 100, 255}, {100, 160, 240, 255}, {240, 160, 60, 255},
		{200, 100, 240, 255}, {100, 220, 220, 255}, {240, 100, 180, 255},
	}
	return colores[int(id)%len(colores)]
}

// Layout fija el tamaño lógico de la pantalla.
func (g *Juego) Layout(anchoExt, altoExt int) (int, int) { return Ancho, Alto }

// errCerrar cierra la ventana de forma controlada (Ebitengine termina RunGame).
var errCerrar = fmt.Errorf("cerrar ventana")

// --------- pantalla de selección de servidores (modo modoSeleccion) ---------

const (
	selFilaAltura = 40
	selFilaY0     = 120
	selFilaX      = 40
	selFilaAncho  = Ancho - 80
)

// updateSeleccion maneja el clic sobre un servidor. Al elegir, conecta y cambia
// a modo juego en la MISMA ventana (no se abre otra).
func (g *Juego) updateSeleccion() error {
	if ebiten.IsKeyPressed(ebiten.KeyEscape) {
		return errCerrar
	}
	apretado := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	if apretado && !g.mouseAntes {
		_, my := ebiten.CursorPosition()
		for i := range g.filas {
			y := selFilaY0 + i*selFilaAltura
			if my >= y && my < y+selFilaAltura-6 {
				g.conectarA(g.filas[i].Addr())
				break
			}
		}
	}
	g.mouseAntes = apretado
	return nil
}

// conectarA intenta conectarse al servidor elegido y pasa a modo juego. Si falla,
// pasa a modo error. En ningún caso cierra la ventana.
func (g *Juego) conectarA(addr string) {
	cli, err := client.Conectar(addr, g.nombre, client.Eventos{})
	if err != nil {
		g.errMsg = err.Error()
		g.m = modoError
		return
	}
	if g.esc != nil {
		g.esc.Detener() // ya no necesitamos seguir escaneando
		g.esc = nil
	}
	g.cli = cli
	g.m = modoJuego
}

func (g *Juego) drawSeleccion(pantalla *ebiten.Image) {
	_ = cargarAssets(g.tema.Pack) // asegura tiles y fuente

	// Fondo de pasto (mismo que el lobby) para dar contexto visual.
	if g.fondoLobby == nil {
		g.fondoLobby = g.construirFondoLobby()
	}
	pantalla.DrawImage(g.fondoLobby, nil)

	escribir(pantalla, "CAPTURA LA BANDERA", Ancho/2, 26, tamGrande, color.RGBA{255, 220, 150, 255}, true)
	escribir(pantalla, "Elegí un servidor (clic). La lista se actualiza sola.", Ancho/2, 68, tamChico, color.RGBA{220, 220, 230, 255}, true)
	escribir(pantalla, fmt.Sprintf("Jugás como: %s     (Escape para salir)", g.nombre), Ancho/2, 90, tamChico, color.RGBA{180, 220, 190, 255}, true)

	if g.esc != nil {
		g.filas = g.esc.Lista()
	}
	if len(g.filas) == 0 {
		escribir(pantalla, "Buscando servidores en la red...", Ancho/2, selFilaY0+20, tamMedio, color.White, true)
		return
	}

	mx, my := ebiten.CursorPosition()
	for i, sv := range g.filas {
		y := selFilaY0 + i*selFilaAltura
		encima := mx >= selFilaX && mx <= selFilaX+selFilaAncho && my >= y && my < y+selFilaAltura-6

		fondo := color.RGBA{34, 34, 44, 230}
		if encima {
			fondo = color.RGBA{50, 100, 74, 240}
		}
		vector.DrawFilledRect(pantalla, float32(selFilaX), float32(y),
			float32(selFilaAncho), float32(selFilaAltura-6), fondo, true)

		estado := "esperando"
		estadoCol := color.RGBA{160, 210, 255, 255}
		if sv.State == protocol.StateRunning {
			estado = "jugando"
			estadoCol = color.RGBA{255, 200, 120, 255}
		}
		fy := float64(y) + 10
		escribir(pantalla, recortarUI(sv.ServerName, 22), selFilaX+12, fy, tamChico, color.White, false)
		escribir(pantalla, sv.Addr(), selFilaX+300, fy, tamChico, color.RGBA{200, 205, 215, 255}, false)
		escribir(pantalla, estado, selFilaX+480, fy, tamChico, estadoCol, false)
		escribirDer(pantalla, fmt.Sprintf("%d/%d", sv.PlayerCount, sv.MaximumPlayers), float64(selFilaX+selFilaAncho)-12, fy, tamChico, color.White)
	}
}

func recortarUI(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
