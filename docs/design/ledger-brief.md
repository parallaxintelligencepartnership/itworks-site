# itworks.dev / THE LEDGER

**Concept key:** `ledger`
**Lens:** screen native, dense, full bleed, no cards, no rounded anything. Big honest
numbers, rules and tabular data as texture. Deliberate rawness as confidence.
Saturated where it counts.

**One sentence:** the site is not a marketing page with a table on it, it *is* the
table, printed edge to edge on a saturated teal ink field, and the badge is treated
as a live instrument rather than a decoration.

---

## 1. Palette

Everything is authored in oklch. Hex values are the sRGB fallbacks and the exact
contract colors. The field is a saturated teal ink, not neutral dark, not paper.
Three chromatic accents carry meaning, so this is not near monochrome with one accent.

| Token | oklch | approx hex | job |
|---|---|---|---|
| `--field` | `oklch(0.205 0.049 205)` | `#08252c` | main ground |
| `--field-band` | `oklch(0.243 0.050 205)` | `#0e2e36` | alternating ledger band |
| `--field-deep` | `oklch(0.165 0.042 205)` | `#041d23` | masthead, wall, sticky bars |
| `--rule` | `oklch(0.395 0.052 205)` | `#2c5661` | hairline rules and column grain |
| `--rule-strong` | `oklch(0.600 0.080 199)` | `#4d8b9b` | section rules, 4px heavy rules |
| `--chalk` | `oklch(0.960 0.014 200)` | `#eaf6f8` | body and headline text |
| `--chalk-dim` | `oklch(0.775 0.028 205)` | `#adc7cd` | secondary text, still AA on field |
| `--acid` | `oklch(0.900 0.198 122)` | `#b4f000` | structure, numerals, install band |
| `--flare` | `oklch(0.690 0.208 31)` | `#ff5a3c` | open critical, and only that |
| `--sun` | `oklch(0.830 0.163 86)` | `#f3b13a` | the day 30 mark on the ruler |

Wide gamut note: on a P3 display `--acid` and `--flare` render outside sRGB, which is
the point of the counter trend. Both stay legible when clamped to sRGB.

### Badge plate colors (fixed by contract, never themed, never tokenized away)

| state | plate | text | contrast |
|---|---|---|---|
| current | `#1a7f37` | `#ffffff` | 5.08:1, AA |
| stale | `#d29922` | `#1f1f1f` | 6.4:1, AA |
| critical open | `#b3261e` | `#ffffff` | 6.57:1, AA |
| left label plate | `#1f1f1f` | `#ffffff` | 15.3:1, AAA |

The whole badge is a fixed 326 x 20 box for all three states, so badges line up in a
column on the wall and in a README stack. The state plate is 250 wide starting at x=76.

### State never rides on hue

* green reads `0 critical open`
* amber reads `0 critical open · stale`, the word is inside the plate
* red reads `1 critical open`, the number itself is the signal

On the wall each row also carries a text status (`state: current` / `state: stale, the
audit is over 30 days old` / `state: critical open, and it stays red until that finding
is closed`) and a left marker bar with a different **pattern** per state: solid for
current, 45 degree hatch for stale, horizontal bars for critical. Color, word, number
and pattern all agree. Remove color entirely and the page still reads correctly.

---

## 2. Type and self host plan

**Archivo Variable** (OFL) for everything structural. Two axes are used as design
tools, not decoration: `wdth` 62..125 and `wght` 100..900.

* wordmark: `wdth 118, wght 860`, `clamp(3.1rem, 15.5vw, 13rem)`, tracking -0.04em
* section headings: `wdth 112, wght 780`
* statement headline: `wdth 96, wght 760` (narrower at large size so long lines hold)
* big numerals: `wdth 104, wght 800` with `font-variant-numeric: tabular-nums lining-nums`

**JetBrains Mono Variable** (OFL) for machine facts only: install commands, dates,
column keys, source labels, the ruler axis. Mono is used where the content is literally
machine output. It is never used as an uppercase eyebrow label, and it is never the
identity of the page.

**Self host:**

1. Subset both variable woff2 to `latin` plus the middle dot U+00B7 with `pyftsubset`,
   keeping the `wdth` and `wght` axes intact (`--drop-tables+=DSIG --flavor=woff2`).
   Result is roughly 34 KB (Archivo) and 30 KB (JetBrains Mono).
2. Embed via `//go:embed assets/fonts/*.woff2` in the single Go binary, served from
   `/f/` with `Cache-Control: public, max-age=31536000, immutable` and a content hash
   in the filename.
3. `@font-face` with `font-display: swap`, `font-weight: 100 900`, `font-stretch: 62% 125%`,
   `unicode-range` on the latin subset, plus `<link rel="preload" as="font" crossorigin>`
   for the two files.
4. `font-synthesis-weight: none` so a missing axis never fakes a weight.
5. Fallback stack is a real metric match: Helvetica Neue / Arial for Archivo,
   ui-monospace / SF Mono / Menlo for the mono.

The mock uses a Google Fonts link for preview only. Production uses no third party
origin, which keeps the CSP as tight as the no script rule implies.

**Fluid type:** every size is a `clamp()` on a viewport term. No breakpoint ever changes
a font size on its own.

---

## 3. Layout system

**Full bleed, no container, no max width.** Content runs to the gutter and the gutter is
`--gut: clamp(16px, 2.2vw, 40px)`. At 400 px that resolves to exactly 16 px.

* The page ground carries a `repeating-linear-gradient` of 8 vertical hairlines at 12.5%
  intervals. That is the ledger grain, drawn in CSS, no image, no grain texture.
* Sections are stacked full bleed bands separated by 1 px hairlines and 4 px heavy rules.
  There are no cards, no shadows, no radii. `border-radius: 0` is set globally on
  `*, ::before, ::after` so nothing can sneak in.
* **The wall** is a real 9 column CSS grid:
  `14px | 1.5fr | 2.1fr | 8.5rem | 8.5rem | 4.2rem x4`
  (marker, name, description, source, audit, found, fixed, accepted, critical).
  Every row repeats the same template so the numbers form true columns. The counts are
  wrapped in a `display: contents` element so they join the parent grid at wide widths
  and become their own 4 column block at narrow widths.
* **Container queries** drive the wall, not media queries. `.wall` is
  `container-type: inline-size`; below 900px of container the column header hides, rows
  collapse to a two column stack, and each number grows a small key label above it.
  Because it is a container query, the wall behaves correctly if it is ever embedded in
  a narrower column.
* **Sticky ledger bars:** a 33 px top bar, and the wall column header sticks beneath it
  at `top: 33px` so the column names stay with the data.
* **400 px:** every band is `padding-inline: 16px`, `body` is `overflow-x: clip`, the
  scroll ruler lane is `overflow: clip` with a `min-inline-size` floor, all grid tracks
  use `minmax(0, ...)`, headlines use `overflow-wrap: break-word`, and the badge SVGs
  drop to 15 px tall (245 px wide) which clears the 368 px content box.

---

## 4. Motion plan

Every animation is `transform` or `opacity` only. Nothing animates layout, color,
filter, or box shadow. All of it is scroll progress, none of it is time based, so
nothing moves unless the reader moves.

All scroll driven rules live inside
`@supports (animation-timeline: view())` nested in
`@media (prefers-reduced-motion: no-preference)`.
Outside that block every element is already in its final state, so a browser without
scroll timelines gets a correct static page, not a blank one.

1. **Row ingest.** Ledger lines (`.leg`, `.plate-row`, `.field`, `.cmd`, `.row`) rise
   14 px and fade in on `animation-timeline: view()`, range `entry 6% cover 26%`.
   Translate3d plus opacity, one composited layer each.
2. **Rule sweep.** The 4 px accent rule at the top of the statement, install and payload
   bands `scaleX(0 -> 1)` from the left as the band enters. Transform only, and the rule
   is a `::before`, so no layout is touched.
3. **Signature: the decay strip.** See below.
4. **Row read.** Hovering or focusing a wall row scales its left marker bar
   `scaleX(2.2)` and drops the other rows' children to `opacity: .45` via
   `.wall:has(.row:hover) .row:not(:hover) > *`. The dim targets children specifically
   because the ingest animation owns the row's own opacity with `fill: both`.
5. **Popover.** The `How the plate is chosen` panel is a native `popover` positioned with
   `anchor-name` / `position-anchor` / `position-area` and `position-try-fallbacks`.
   It enters from `@starting-style` with `opacity` and an 8 px `translateY`, using
   `transition-behavior: allow-discrete` on `display` and `overlay`. No script, keyboard
   and Escape work natively, and there is an `@supports not (anchor-name)` fallback that
   docks it to the bottom of the viewport.
6. **Filter.** Four radio inputs plus `:has(:checked)` on the wall filter the ledger to
   everything / current / stale / critical open. Real interaction, fully keyboard
   operable, zero script.

### Reduced motion

`@media (prefers-reduced-motion: reduce)` sets `animation: none !important` and
`transition: none !important` on everything, and then does the harder work of making the
static state *say the same thing*:

* the decay strip stops being a scrub. The runner becomes a grid and the green and amber
  plates are shown stacked, both at full opacity, as a before and after pair.
* the tick ruler and the odometer are hidden, because they are meaningless frozen.
* a sentence that is `display: none` by default becomes visible in their place:
  "The two plates above are the same badge at day 0 and at day 30. Motion is off because
  your system asks for it."
* the marker hover scale is pinned off.

No information exists only in motion.

---

## 5. The signature moment: the decay strip

A full bleed band with a 60 day ruler across it. One real badge sits in the lane. As the
band passes through the viewport, scroll progress (`view(block)`, range
`entry 92% exit 8%`, so it scrubs as it passes and nothing is ever pinned) drives three
synchronized animations:

1. the badge translates along the lane, `translate3d(0 -> calc(100cqw - 100%))` using
   container query units on the lane so the travel is exact at any width;
2. the green plate and the amber plate cross fade with a hard cut at 50% of the range,
   authored as `0%, 49.6% { opacity: 1 } 50%, 100% { opacity: 0 }`. It is a jump, not a
   blend, because a badge does not gradually become stale, it flips on day 30;
3. an odometer of day numbers translates on Y with `steps(7, jump-none)`, so the reader
   sees `0 10 20 30 40 50 60` click over as the plate moves. It is a masked strip of
   `<b>` elements inside a fixed height window, so it is pure transform.

The lane is sized `calc(100% - 326px)` (the plate width), and the day 30 mark sits at the
exact midpoint of that lane, so the acid pointer line on the plate's left edge lines up
with the mark at the moment the plate flips. The mechanism is honest: the pointer is the
edge that is actually being measured.

Below it, the red branch is stated flat, with a static red plate: time is only one of the
two triggers, and an open critical beats age on any day including day 1.

**Why this is the moment:** the brief said nobody has made a trust artifact feel alive
without JavaScript. This does not decorate the badge, it *operates* it. The reader
watches the rule execute on a real plate, which is a far better explanation of the rule
than the paragraph next to it.

---

## 6. How the honest badge is presented

The badge is never dressed up. It is a flat two plate SVG, hard 90 degree corners, 20 px
tall, Verdana at 11 px, the same shape the rest of the ecosystem already recognizes. No
rounding, no gloss, no shadow, no animation on the badge itself.

It appears four times, each time doing a different job:

1. **Operated** in the decay strip, where the rule runs in front of you.
2. **Specified** in the three plate rows, each with its exact hex pair and its contrast
   reasoning written out in plain text next to it. The section says, verbatim,
   "The badge can look bad on purpose."
3. **Explained** in the popover, as three ordered rules with red as an absolute override.
4. **Applied** on the wall, printed under each entry at the same fixed width, next to a
   written state line, so the badge and the raw numbers are always readable together and
   can be checked against each other.

The wall deliberately shows a stale one and a red one in the preview. A wall that only
showed green badges would not be a ledger, it would be a brochure.

---

## 7. Content rules honored

* Copy is plain English, US spelling, no dashes anywhere outside ISO dates, and the
  phrase is never used. Verified by stripping tags and grepping (see the report).
* No site wide statistics are invented. The wall header describes columns and sort order
  only. No totals, no averages, no counts of apps.
* The three sample entries are internally consistent: found = fixed + accepted + still
  open, and the still open figure matches the critical count where relevant.
* The two install commands appear verbatim, each with one line of context.
* All eight published fields are listed, followed by the explicit statement that nothing
  else is sent, and the statement that the site never runs a model.

---

## 8. Why this would be copied in 2027

Three things here are new, and all three are portable.

**A trust artifact that performs its own rule.** Every badge on the web today is a static
PNG or SVG that asserts a state. This one is scrubbed through its own state machine in
front of the reader, with a hard cut on the threshold day, using scroll progress as the
only input. It is the first time the explanation of a badge and the badge itself are the
same object, and it needs no script, so it survives a content security policy that blocks
JavaScript outright. Any project with a status badge, a certificate, an SLA, a freshness
guarantee or an expiry date can copy the pattern directly, and the developer tooling
world is full of those.

**A ruler measured in container query units.** The travel lane is sized as
`calc(100% - <plate width>)` and the badge is moved by `calc(100cqw - 100%)`, which means
the ruler, the threshold mark and the moving object stay in exact agreement at 400 px and
at 1440 px with no breakpoints and no measurement code. That is the technique people have
been reaching for a scroll library to do, and it turns out to be four declarations.

**Accessibility as the aesthetic rather than a tax on it.** State is carried four ways at
once, color, word, number and pattern, and the reduced motion branch is not a switch that
turns the idea off but a second, static composition that makes the same argument. The
tabular density, the hatched markers and the plain word "stale" inside the amber plate
are not compliance artifacts bolted onto a design, they are the design. That is the move
that will look obvious in 2027 and does not yet look obvious now: the saturated,
data dense, honest ledger reads as *more* confident than the polished card wall, because
it is willing to print the red row at full size and let you filter straight to it.

What it is deliberately not: not paper, not grain, not a broadside, not a type specimen,
not a bento grid, not glass, not a purple gradient, not a dark SaaS page with a glow, and
not a pinned scroll reel. It is a screen native ledger that runs.
