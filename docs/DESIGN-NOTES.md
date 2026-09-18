# Design notes

## 2026-09-11: Matt picked Ledger (round two)

Chosen: the Ledger concept (dark, chartreuse accent, dense tabular ledger, badge that rides a 60 day ruler and flips on day 30). Mock: scratchpad design2/ledger/index.html and BRIEF.md from the round two workflow; copy both into docs/design/ before building.

Matt's notes on the mock, verbatim intent, to fix before or during the build:
1. Spacing and alignment are terrible. Body sections read as if formatted for a mobile device and are not dynamically resized by screen size or device. Real responsive layout at every width is required, not a mobile column stretched to desktop.
2. The badges themselves look terrible. Redesign the badge rendering (the on page SVG and the served SVG) so it looks good, while keeping AA contrast and the state carried by shape and word as well as color.
3. The prose that says the badge is supposed to look bad ("the badge can look bad on purpose" and the explanation around it) has to go. The badge states remain honest; the copy stops apologizing for them.
4. Fix the hero wrap: "itworks.build" broke onto two lines at 1440 px in the mock.

Status: noted only. Build not started; Matt said "just note this".

## 2026-09-11, later: Matt's second round of notes on Ledger
5. Far too much copy. "So much slop writing on that page it's unreadable. It's like 4 pages of reading content; nobody is going to sit around and read a novel." Cut the landing page to a fraction: short lines, no paragraphs, let the ledger and the badge do the talking.
6. Needs a deeper showcase pass, "WOW mode": fluid, smooth as butter animations and scrolling, extra design touches leaning on new concepts for the modern web. Refine Ledger, do not tear it down. Ledger remains the choice.
7. Plugin name: Matt is considering renaming vibecheck if the name is already in wide use elsewhere. Research pending; his call. Decided 2026-09-11: renamed to itworks.

## 2026-09-17: Round 3, the public notice board

Matt's call on the live Ledger build: it "looks like a basic AI SaaS site". Three
round three concepts were drawn (docs/design/round3/: notice, inspection, bench);
he picked the public notice board for the whole site, and mixing it with the
inspection sheet was rejected as "out of place". The mocks in
docs/design/round3/notice are the source of truth; the built pages are captured
in docs/design/round3/notice/impl/ at 1440 and at 390.

### What changed
- Every template is rewritten. The dark chartreuse field, the ledger rows, the
  popover rule, the CSS only wall filter and the scroll snap are gone.
- The site is a bill pasted on a civic board: a notice header line (number and
  posted date), the wordmark printed with a misregistered acid second pass, the
  claim in condensed uppercase, Board A (the state word on split flap cells),
  Board B (the 60 day posted term), the install lines as tear off tabs under a
  perforated rule, the published register, and the "never runs a model" stamp.
- The wall is the register: big slot numbers, name and summary, the tier and
  audit date with a 60 day minirule, the four counts, the plate and the state in
  words. Empty is not an error: the board reads blank and slot 001 is ruled off
  with four pins in.
- An entry page is one posted notice: notice number is the entry id, posted date
  is the audit date, the badge is shown as the notice with the markdown to copy,
  the figures are posted large, the term rule carries this entry's day, and the
  state word sits on the flaps. 404 is a torn, empty slot.
- The landing keeps a three notice preview of the register, which the Ledger
  landing also carried; the mock had no such section, but that is data and data
  is not dropped. The wall page filter chips are gone with the mock, since they
  were interface, not record.

### Palette tokens
Printed ink on posted stock. No dark field, no card, no gradient, no radius, no
hover lift anywhere in the stylesheet.

| Token | Value | What it is |
|---|---|---|
| --stock / --stock-2 / --stock-3 | #e9e4d6 / #ddd7c4 / #d2cbb5 | the three paper tones sections alternate between |
| --board / --board-2 | #16180f / #0b0c06 | the departure board, and the inside of a flap cell |
| --ink / --ink-dim | #101208 / #4c4e3e | the one printing ink, and its lighter pass |
| --hair | #a9a693 | hairline rules only, never text |
| --acid | #b4f000 | the second pass: the wordmark slip, the bar edge, the command highlighter |
| --flare | #cf3611 | rules, pins, the day pip, the wordmark dot |
| --flare-ink | #9e2a0c | the same red where it has to carry small text |
| --sun | #d29922 | day 30 on the rule, the stale flap |
| --board-ink / --board-dim | #c3c5ae / #90937c | body and label text on the board |
| --plate-green / --plate-amber / --plate-red | #1a7f37 / #d29922 / #b3261e | badge plates, fixed by contract with internal/badge |

### Contrast, measured (WCAG 2.1, AA needs 4.5 for text, 3.0 for large text)
- ink on stock 14.9, on stock-2 13.1, on stock-3 11.6
- ink-dim on stock 6.7, on stock-2 5.9, on stock-3 5.3
- flare-ink on stock 5.9, on stock-2 5.2, on stock-3 4.6
- stock on board 14.1, board-ink on board 10.2, board-dim on board 5.5
- acid on board 13.2, ink on acid (the install tabs) 13.9
- flap letters on board-2: current #6ee08f 11.9, stale --sun 7.8, critical #ff7a5e 7.7
- ruler zones: white on plate-green 5.1, #1f1f1f on plate-amber 6.5
- --flare at 4.0 on stock never carries small text: it is rules, pins, the pip and
  the wordmark dot, which is display size. That is why --flare-ink exists.

### Split flap mechanics
The state word is one cell per letter, built in Go (view.go, `newFlap`) so a
space is a gap between cells and a screen reader is handed the whole word
through `aria-label` instead of loose letters. A cell is a dark plate with a
hairline hinge across its middle and two pin dots on its edges. Under
`prefers-reduced-motion: no-preference` only, each cell clicks over once on load
with `rotateX(-92deg)` to `0`, staggered 40ms per cell, up to 14 cells. There is
no JavaScript: the animation is the whole mechanism, and with reduced motion the
letters are simply there.

### Footer rules
Decided 2026-09-17 (.itworks/DECISIONS.md), pinned by
`TestFooterCarriesTheRecordAndFollowsItsLinks`:
- The full legal name is stated once, in the identity block: Parallax
  Intelligence Partnership, LLC, Battle Creek, Michigan, and the mailto. The
  copyright line therefore reads "© 2026 itworks.build", so the name is not
  printed twice.
- Sister sites sit under "Also posted by this office": parallaxintelligence.ai,
  parallaxintelligence.digital, stillpub.app, postmortem.report. The bare word
  Parallax is never a heading; other entities share the name.
- "The code, on GitHub" lists the org, then itworks, itworks-site, weatherdesk,
  openscan-hub, pulse-libre, frigateios.
- "The record kept here" lists the wall, the JSON feed, and the embed
  instructions in the README.
- Every outbound link in the footer is followed. `rel="noopener"` only, never
  nofollow: the point of the footer is to pass link equity to Matt's properties.
  Submitter chosen repo links keep `rel="nofollow noopener noreferrer"`, which is
  the B3 finding fix and a different question.
- No LinkedIn anywhere. "This site never runs a model." is printed in the footer
  of every page.

### Head and crawlability
base.html now carries a canonical link, a per page description, Open Graph
(title, description, url, type, site_name) and twitter:card, plus one
application/ld+json block holding an Organization and a WebSite. The JSON is
built with encoding/json in view.go (`siteJSONLD`) and handed to the template as
`template.JS`, so no page hand writes JSON. The build also writes sitemap.xml
(the landing, the wall, and every entry page with its approval date as lastmod)
and robots.txt.
