// Package assets embebe los sprites del juego (pack CC0 "Tiny Farm" de Kenney,
// www.kenney.nl). Al estar embebidos con go:embed, el binario es autocontenido:
// no hay que distribuir la carpeta assets/ por separado.
//
// El go:embed debe vivir en este mismo directorio porque los patrones no pueden
// referirse a carpetas superiores (no se permite "..").
package assets

import "embed"

// Tiles contiene los 132 tiles individuales de 16×16, nombrados
// Tiles/tile_0000.png .. Tiles/tile_0131.png (orden fila por fila, 12 por fila).
//
//go:embed Tiles/*.png
var Tiles embed.FS

// UI contiene los sprites de interfaz (pack CC0 "UI Pack — Pixel Adventure" de
// Kenney): paneles de madera para el lobby, etc. Son tiles de 32×32 con borde,
// pensados para estirar como panel (9-slice).
//
//go:embed UI/*.png
var UI embed.FS

// Temas contiene la definición de cada mapa: temas/<nombre>/tiles.json y
// temas/<nombre>/distribucion.json. Agregar una carpeta = agregar un mapa (ver
// temas/README.md).
//
//go:embed temas
var Temas embed.FS

// Fonts contiene la tipografía del juego (fuente CC0 "Kenney Pixel").
//
//go:embed Fonts/kenney-pixel.ttf
var Fonts embed.FS

// Packs contiene tilesets alternativos que no vienen como PNG sueltos, sino como
// una sola imagen (spritesheet) más, opcionalmente, un .tsx de Tiled con las
// animaciones. Por ahora: Puny World (overworld, con agua animada).
//
//go:embed packs/punyworld/tileset.png packs/punyworld/tiles.tsx
var Packs embed.FS
