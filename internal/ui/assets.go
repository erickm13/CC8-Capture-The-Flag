package ui

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/png" // registra el decodificador PNG para image.Decode
	"sync"

	"ctf/assets"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// tiles guarda los tiles del pack activo ya decodificados, indexados por número.
// animMap mapea índice de tile → secuencia de frames (para tiles animados).
// El pack se fija en la primera llamada a cargarAssets (un tema por sesión).
var (
	tiles     []*ebiten.Image
	animMap   map[int][]int
	cargarUna sync.Once
	cargarErr error
)

// cargarAssets carga una sola vez el pack de tiles indicado, la interfaz y la
// fuente. Es idempotente y seguro de llamar en cada cuadro.
func cargarAssets(pack string) error {
	cargarUna.Do(func() {
		if tiles, animMap, cargarErr = cargarPack(pack); cargarErr != nil {
			return
		}
		if cargarErr = cargarUI(); cargarErr != nil {
			return
		}
		cargarErr = cargarFuente()
	})
	return cargarErr
}

// tile devuelve el tile por índice, o nil si está fuera de rango.
func tile(i int) *ebiten.Image {
	if i < 0 || i >= len(tiles) {
		return nil
	}
	return tiles[i]
}

// tileAnimado devuelve el frame actual de un tile animado según ms (100 ms por
// frame, como define el .tsx). Si el tile no tiene animación, devuelve el fijo.
func tileAnimado(i int, ms int64) *ebiten.Image {
	fr := animMap[i]
	if len(fr) == 0 {
		return tile(i)
	}
	return tile(fr[int(ms/100)%len(fr)])
}

// Paneles de interfaz (UI pack), cargados una vez junto con los tiles.
var (
	panelClaro  *ebiten.Image
	panelOscuro *ebiten.Image
)

// cargarUI decodifica los paneles de interfaz embebidos. Idempotente; se llama
// dentro de cargarTiles.
func cargarUI() error {
	for _, p := range []struct {
		nombre  string
		destino **ebiten.Image
	}{
		{"UI/panel.png", &panelClaro},
		{"UI/panel_dark.png", &panelOscuro},
	} {
		datos, err := assets.UI.ReadFile(p.nombre)
		if err != nil {
			return fmt.Errorf("leyendo %s: %w", p.nombre, err)
		}
		img, _, err := image.Decode(bytes.NewReader(datos))
		if err != nil {
			return fmt.Errorf("decodificando %s: %w", p.nombre, err)
		}
		*p.destino = ebiten.NewImageFromImage(img)
	}
	return nil
}

// dibujarPanel dibuja src estirado como panel de tamaño (w, h) en (x, y) usando
// 9-slice: las esquinas se mantienen, los bordes y el centro se estiran. Así un
// solo tile con borde sirve como panel de cualquier tamaño sin deformar el marco.
func dibujarPanel(dst, src *ebiten.Image, x, y, w, h float64) {
	if src == nil {
		vector.DrawFilledRect(dst, float32(x), float32(y), float32(w), float32(h),
			color.RGBA{48, 34, 24, 235}, true)
		return
	}
	const s = 8   // inset del borde en píxeles de la fuente (tiles de 32px)
	const f = 3.0 // escala del borde en pantalla
	d := s * f
	sw := float64(src.Bounds().Dx())
	sh := float64(src.Bounds().Dy())

	pieza := func(sx, sy, psw, psh, dx, dy, dw, dh float64) {
		sub := src.SubImage(image.Rect(int(sx), int(sy), int(sx+psw), int(sy+psh))).(*ebiten.Image)
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(dw/psw, dh/psh)
		op.GeoM.Translate(dx, dy)
		dst.DrawImage(sub, op)
	}

	mx := sw - 2*s // franja central de la fuente (horizontal)
	my := sh - 2*s
	dcw := w - 2*d // franja central en destino
	dch := h - 2*d
	// esquinas
	pieza(0, 0, s, s, x, y, d, d)
	pieza(sw-s, 0, s, s, x+w-d, y, d, d)
	pieza(0, sh-s, s, s, x, y+h-d, d, d)
	pieza(sw-s, sh-s, s, s, x+w-d, y+h-d, d, d)
	// bordes
	pieza(s, 0, mx, s, x+d, y, dcw, d)        // arriba
	pieza(s, sh-s, mx, s, x+d, y+h-d, dcw, d) // abajo
	pieza(0, s, s, my, x, y+d, d, dch)        // izquierda
	pieza(sw-s, s, s, my, x+w-d, y+d, d, dch) // derecha
	// centro
	pieza(s, s, mx, my, x+d, y+d, dcw, dch)
}

// dibujarTile pega src en dst con la esquina superior izquierda en (x, y),
// escalado por factor. El filtro por defecto de Ebitengine es nearest, así que
// el pixel-art se mantiene nítido al agrandar.
func dibujarTile(dst, src *ebiten.Image, x, y, factor float64) {
	if src == nil {
		return
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(factor, factor)
	op.GeoM.Translate(x, y)
	dst.DrawImage(src, op)
}

// dibujarTileCentrado pega src centrado en (cx, cy), escalado por factor.
func dibujarTileCentrado(dst, src *ebiten.Image, cx, cy, factor float64) {
	if src == nil {
		return
	}
	w := float64(src.Bounds().Dx())
	h := float64(src.Bounds().Dy())
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(-w/2, -h/2)
	op.GeoM.Scale(factor, factor)
	op.GeoM.Translate(cx, cy)
	dst.DrawImage(src, op)
}
