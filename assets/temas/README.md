# Temas (mapas) del juego

Cada subcarpeta de `temas/` es un **mapa**. El juego los descubre solos: para
agregar uno nuevo, creá una carpeta con su nombre y adentro estos dos archivos.
Al correr con `go run` se reconstruye y lo toma automáticamente (aparece en el
modo aleatorio y en el futuro selector).

```
temas/
  campo/
    tiles.json          ← qué tile es cada cosa
    distribucion.json   ← cuántos y dónde
  <tu-mapa>/            ← una carpeta nueva = un mapa nuevo
    tiles.json
    distribucion.json
```

## `tiles.json`

Los números son **índices de tile** del pack. Mirá `assets/Preview.png` para ver
cuál es cuál (orden fila por fila, 12 por fila; índice = fila×12 + columna).

| Campo         | Qué es                                            |
|---------------|---------------------------------------------------|
| `pack`        | nombre del pack de tiles (informativo)            |
| `pasto`       | tiles de fondo; se alternan por celda             |
| `tierraColor` | color del círculo central, en hex `#rrggbb`       |
| `cultivos`    | sprites esparcidos dentro del círculo             |
| `decoracion`  | árboles/arbustos esparcidos afuera                |
| `props`       | objetos y animales que dan vida al campo (afuera) |
| `edificio`    | matriz de tiles de un edificio (ej: granero)      |
| `jugador`     | sprite del jugador                                |
| `bandera`     | sprite de la bandera                              |

## `distribucion.json`

| Campo          | Qué es                                             |
|----------------|----------------------------------------------------|
| `cantCultivos` | cuántos cultivos dentro del círculo                |
| `cantArboles`  | cuántos árboles/arbustos afuera                    |
| `cantProps`    | cuántos props/animales afuera                      |
| `graneros`     | posiciones `[x, y]` (en píxeles, ventana 800×800)  |
| `semilla`      | semilla del azar; cambiala para otra distribución  |
