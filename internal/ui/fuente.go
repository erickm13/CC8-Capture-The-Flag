package ui

import (
	"bytes"
	"image/color"

	"ctf/assets"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

// Tamaños de texto usados en la interfaz.
const (
	tamChico  = 16
	tamMedio  = 22
	tamGrande = 34
)

// fuenteFuente es la fuente Kenney Pixel ya parseada; caras cachea una GoTextFace
// por tamaño. Se cargan con text/v2 de Ebitengine, sin dependencias extra.
var (
	fuenteFuente *text.GoTextFaceSource
	caras        = map[float64]*text.GoTextFace{}
)

// cargarFuente parsea la tipografía embebida. Se llama desde cargarTiles.
func cargarFuente() error {
	datos, err := assets.Fonts.ReadFile("Fonts/kenney-pixel.ttf")
	if err != nil {
		return err
	}
	f, err := text.NewGoTextFaceSource(bytes.NewReader(datos))
	if err != nil {
		return err
	}
	fuenteFuente = f
	return nil
}

// cara devuelve (y cachea) la fuente al tamaño pedido, o nil si no cargó.
func cara(tam float64) *text.GoTextFace {
	if fuenteFuente == nil {
		return nil
	}
	if f, ok := caras[tam]; ok {
		return f
	}
	f := &text.GoTextFace{Source: fuenteFuente, Size: tam}
	caras[tam] = f
	return f
}

// escribir dibuja texto con la fuente del juego en (x, y). Con centrado=true, x
// es el centro horizontal. Siempre dibuja una sombra oscura detrás para que se
// lea sobre fondos con textura. Si la fuente no cargó, cae en la fuente de debug.
func escribir(dst *ebiten.Image, txt string, x, y, tam float64, col color.Color, centrado bool) {
	f := cara(tam)
	if f == nil {
		ebitenutil.DebugPrintAt(dst, txt, int(x), int(y))
		return
	}
	alin := text.AlignStart
	if centrado {
		alin = text.AlignCenter
	}

	sombra := &text.DrawOptions{}
	sombra.PrimaryAlign = alin
	sombra.GeoM.Translate(x+2, y+2)
	sombra.ColorScale.ScaleWithColor(color.RGBA{0, 0, 0, 170})
	text.Draw(dst, txt, f, sombra)

	op := &text.DrawOptions{}
	op.PrimaryAlign = alin
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(col)
	text.Draw(dst, txt, f, op)
}

// escribirDer dibuja texto alineado a la derecha, terminando en x.
func escribirDer(dst *ebiten.Image, txt string, x, y, tam float64, col color.Color) {
	f := cara(tam)
	if f == nil {
		ebitenutil.DebugPrintAt(dst, txt, int(x)-len(txt)*6, int(y))
		return
	}
	sombra := &text.DrawOptions{}
	sombra.PrimaryAlign = text.AlignEnd
	sombra.GeoM.Translate(x+2, y+2)
	sombra.ColorScale.ScaleWithColor(color.RGBA{0, 0, 0, 170})
	text.Draw(dst, txt, f, sombra)

	op := &text.DrawOptions{}
	op.PrimaryAlign = text.AlignEnd
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(col)
	text.Draw(dst, txt, f, op)
}
