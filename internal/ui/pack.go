package ui

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"image"

	"ctf/assets"

	"github.com/hajimehoshi/ebiten/v2"
)

// cargarPack decodifica un pack de tiles y su tabla de animaciones (índice de
// tile → secuencia de frames). Cada tema declara qué pack usa en su tiles.json.
func cargarPack(nombre string) ([]*ebiten.Image, map[int][]int, error) {
	switch nombre {
	case "", "farm", "Tiny Farm":
		return cargarPackFarm()
	case "puny", "Puny World":
		return cargarPackPuny()
	default:
		return nil, nil, fmt.Errorf("pack de tiles desconocido: %q", nombre)
	}
}

// cargarPackFarm: 132 PNG sueltos de 16×16 (Tiny Farm), sin animaciones.
func cargarPackFarm() ([]*ebiten.Image, map[int][]int, error) {
	const n = 132
	ts := make([]*ebiten.Image, n)
	for i := 0; i < n; i++ {
		nombre := fmt.Sprintf("Tiles/tile_%04d.png", i)
		datos, err := assets.Tiles.ReadFile(nombre)
		if err != nil {
			return nil, nil, fmt.Errorf("leyendo %s: %w", nombre, err)
		}
		img, _, err := image.Decode(bytes.NewReader(datos))
		if err != nil {
			return nil, nil, fmt.Errorf("decodificando %s: %w", nombre, err)
		}
		ts[i] = ebiten.NewImageFromImage(img)
	}
	return ts, nil, nil
}

// cargarPackPuny: un solo spritesheet 16×16 (27 columnas) que se recorta en
// tiles, más las animaciones (agua) leídas del .tsx de Tiled.
func cargarPackPuny() ([]*ebiten.Image, map[int][]int, error) {
	datos, err := assets.Packs.ReadFile("packs/punyworld/tileset.png")
	if err != nil {
		return nil, nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(datos))
	if err != nil {
		return nil, nil, err
	}
	recortable, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	})
	if !ok {
		return nil, nil, fmt.Errorf("el tileset de puny no soporta SubImage")
	}

	const tam = 16
	rec := img.Bounds()
	cols := rec.Dx() / tam
	filas := rec.Dy() / tam
	ts := make([]*ebiten.Image, cols*filas)
	for f := 0; f < filas; f++ {
		for c := 0; c < cols; c++ {
			r := image.Rect(rec.Min.X+c*tam, rec.Min.Y+f*tam,
				rec.Min.X+(c+1)*tam, rec.Min.Y+(f+1)*tam)
			ts[f*cols+c] = ebiten.NewImageFromImage(recortable.SubImage(r))
		}
	}

	anim, err := cargarAnimPuny()
	if err != nil {
		return nil, nil, err
	}
	return ts, anim, nil
}

// cargarAnimPuny lee las animaciones del .tsx: cada <tile> con <animation>
// define su secuencia de frames (ids de tile).
func cargarAnimPuny() (map[int][]int, error) {
	datos, err := assets.Packs.ReadFile("packs/punyworld/tiles.tsx")
	if err != nil {
		return nil, err
	}
	var doc struct {
		Tiles []struct {
			ID     int `xml:"id,attr"`
			Frames []struct {
				TileID int `xml:"tileid,attr"`
			} `xml:"animation>frame"`
		} `xml:"tile"`
	}
	if err := xml.Unmarshal(datos, &doc); err != nil {
		return nil, err
	}
	anim := map[int][]int{}
	for _, t := range doc.Tiles {
		if len(t.Frames) == 0 {
			continue
		}
		fr := make([]int, len(t.Frames))
		for i, f := range t.Frames {
			fr[i] = f.TileID
		}
		anim[t.ID] = fr
	}
	return anim, nil
}
