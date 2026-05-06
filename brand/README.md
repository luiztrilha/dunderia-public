# MaestrIA Brand Assets

Official logo, icon exports, and usage guidance for the MaestrIA brand.

## The Logo

The MaestrIA mark is a pixel-grid `M` with a small conductor baton accent. It keeps the old runtime's crisp favicon discipline, but shifts the personality from office parody to orchestration: dark control-room base, warm maestro gold, and a precise cyan signal line.

It is built from a 16-unit grid with `shape-rendering="crispEdges"`. Small exports stay sharp in browser tabs and task bars, while larger PNGs can be used in docs, app shells, and release imagery.

### Primary - `maestria-logo.svg`

![Primary logo](png/maestria-logo-128.png)

Gold frame, midnight interior, gold `M`, and cyan baton. Use this everywhere by default: browser tabs, app icons, social avatars, READMEs, docs, and release assets.

### Inverted - `maestria-logo-inverted.svg`

![Inverted logo](png/maestria-logo-inverted-128.png)

Midnight `M` stamped on a gold field. Use this only when the primary mark would lose contrast or when a single-color badge reads better.

## Colors

| Token | Hex | Role |
|-------|-----|------|
| `--maestro-gold` | `#F5C542` | Primary brand accent |
| `--midnight` | `#0E1726` | Primary background |
| `--signal-cyan` | `#43D5C8` | Baton and live-system accent |
| `--paper` | `#F7F3E8` | Warm foreground text |

## Files

```
brand/
  maestria-logo.svg
  maestria-logo-inverted.svg
  png/
    maestria-logo-16.png
    maestria-logo-32.png
    maestria-logo-64.png
    maestria-logo-128.png
    maestria-logo-180.png
    maestria-logo-192.png
    maestria-logo-256.png
    maestria-logo-512.png
    maestria-logo-1024.png
    maestria-logo-inverted-*.png
```

Legacy `wuphf-logo*` files remain for historical compatibility. New public surfaces should use the MaestrIA assets.

## Do

- Use the SVG whenever possible.
- Keep the three brand colors exact.
- Let the mark sit on a solid or low-noise background.
- Use the primary logo unless contrast requires the inverted mark.

## Do Not

- Do not recolor the logo outside the defined palette.
- Do not blur, round, stretch, or rotate it.
- Do not place the primary mark on a gold field; use the inverted version instead.
