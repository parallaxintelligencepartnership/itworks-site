# Site icon

The favicon is the "it" monogram: two chalk stems, one crossbar, and the dot of
the i in acid, on the field plate. Matt picked it on 2026-09-17 from round 2.
Source of truth is `round2/it.svg`; `web/static/favicon.svg` is a copy of it,
and the two PNGs (`favicon-32.png`, `apple-touch-icon.png` at 180 px) are
rasterized from the same shapes with a stdlib Python script, 8x supersampled,
because no SVG rasterizer is installed on the build machine. Regenerate all
three together if the mark changes.

- `round1/`: three rect grid marks derived from the badge and ruler, rejected.
- `round2/`: the full stop, the loop, and the "it" monogram, with `preview.html`
  showing each at 16 to 128 px in a mock tab on white, field, and grey.
