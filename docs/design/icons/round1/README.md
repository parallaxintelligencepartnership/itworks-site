# Icon candidates

Three marks for itworks.build, all drawn from the Ledger system, all geometric,
none of them lettering. Each is a 64 unit viewBox, three or four filled shapes,
no strokes, no gradients, nothing thinner than 4 units (1/16 of the box). The
rectangles sit on a 4 unit grid, so at 16 px every edge lands on a whole pixel
and nothing goes soft. Each carries a `<style>` block with a
`prefers-color-scheme: dark` rule.

## candidate-a.svg, the day 30 mark
The 60 day review ruler seen end on: the `--acid` elapsed lane, the
`--rule-strong` scale below it, and the `--sun` day 30 mark standing across both
at the midpoint.
It is the site's one idea in three rectangles, the moment a badge flips from
current to stale, and the sun bar is the only tall shape so the silhouette holds
at 16 px.
Dark scheme: the ground lifts to `--field-band` and the scale goes to `--chalk`,
because `--rule-strong` is too quiet against a dark tab strip.

## candidate-b.svg, the plate and its edge
The two plate badge squared off: the dark label plate as the ground, the `--sun`
state plate filling the right, and the `--acid` pointer standing on the plate's
left edge, which is the edge the ruler actually measures.
It is the artifact that leaves the site, the thing people will already have seen
in a README, and the acid pointer keeps it from reading as a plain block.
Dark scheme: the ground lifts to `--field-band`; the sun plate and the acid
pointer carry the mark on their own if the ground disappears into the tab.

## candidate-c.svg, the stale hatch
The left marker bar pattern: the 45 degree hatch that means stale on the wall,
run across the whole plate in the amber plate color.
It is the clearest proof of the rule that state never rides on hue, and diagonals
read at any size because nothing has to resolve.
Dark scheme: the ground lifts to `--field-band` and the hatch brightens from the
amber plate to `--sun`, which holds up better against a dark tab.

## Recommendation
**candidate-a.svg**, wired in as `web/static/favicon.svg`: it is the only one of
the three that carries the site's actual argument (a review has an age and a
threshold), it holds an asymmetric, unmistakable silhouette at 16 px, and unlike
candidate-b it does not shrink a 326 x 20 badge into a square it was never drawn
for.

## Raster icons
`web/static/favicon-32.png` (32 px) and `web/static/apple-touch-icon.png`
(180 px) are rendered from `favicon.svg` in the light scheme colors, fully
opaque. No rasterizer from the usual set is on this machine: python3 has neither
cairosvg nor Pillow, node has neither sharp nor resvg, and rsvg-convert,
ImageMagick and Inkscape are absent. Nothing was installed. Because the mark is
four axis aligned rectangles, the two PNGs were rendered with a short
standard library script (xml.etree to read the rects, 4x supersampling, zlib to
write the PNG). Regenerating them by hand is fine; keeping them in step with the
SVG is the only rule.

## preview.html
Open `preview.html` in a browser to see all three at 16, 32, 64 and 180 px on a
white strip and on a `#041d23` strip. The strips show the same file twice; the
`prefers-color-scheme` rule inside each SVG follows the operating system
setting, not the strip it sits on, so switch the system appearance to check the
dark branch.
