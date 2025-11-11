# Celestial Bloom Tower Defense

Celestial Bloom is a hand-crafted tower defense experience written entirely in Go and powered by the [Ebiten](https://ebitengine.org/) game framework. Command a constellation of living turrets, paint the arena with prismatic energy, and survive increasingly punishing waves of invaders.

## Features

- **Vibrant visuals** – layered nebula backgrounds, animated sparkles, and glowing path effects set the stage for the action.
- **Tactile tower placement** – click to place or right click to upgrade, with real-time placement feedback and scalable range auras.
- **Progressive waves** – five bespoke enemy waves culminating in a hulking boss encounter.
- **Responsive gameplay** – smooth projectile arcs, pulsing enemies, and animated UI to keep the battlefield feeling alive.

## Requirements

- Go 1.21+

On the first run the module tooling will fetch Ebiten and font dependencies automatically via `go mod tidy` or `go run`.

## Running the game

```bash
go run ./...
```

The window is resizable and defaults to 1024×768. Place towers with the left mouse button, upgrade with the right mouse button, and defend your celestial garden from every wave.
