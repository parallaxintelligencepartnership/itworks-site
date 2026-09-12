# Design notes

## 2026-09-11: Matt picked Ledger (round two)

Chosen: the Ledger concept (dark, chartreuse accent, dense tabular ledger, badge that rides a 60 day ruler and flips on day 30). Mock: scratchpad design2/ledger/index.html and BRIEF.md from the round two workflow; copy both into docs/design/ before building.

Matt's notes on the mock, verbatim intent, to fix before or during the build:
1. Spacing and alignment are terrible. Body sections read as if formatted for a mobile device and are not dynamically resized by screen size or device. Real responsive layout at every width is required, not a mobile column stretched to desktop.
2. The badges themselves look terrible. Redesign the badge rendering (the on page SVG and the served SVG) so it looks good, while keeping AA contrast and the state carried by shape and word as well as color.
3. The prose that says the badge is supposed to look bad ("the badge can look bad on purpose" and the explanation around it) has to go. The badge states remain honest; the copy stops apologizing for them.
4. Fix the hero wrap: "itworks.dev" broke onto two lines at 1440 px in the mock.

Status: noted only. Build not started; Matt said "just note this".

## 2026-09-11, later: Matt's second round of notes on Ledger
5. Far too much copy. "So much slop writing on that page it's unreadable. It's like 4 pages of reading content; nobody is going to sit around and read a novel." Cut the landing page to a fraction: short lines, no paragraphs, let the ledger and the badge do the talking.
6. Needs a deeper showcase pass, "WOW mode": fluid, smooth as butter animations and scrolling, extra design touches leaning on new concepts for the modern web. Refine Ledger, do not tear it down. Ledger remains the choice.
7. Plugin name: Matt is considering renaming vibecheck if the name is already in wide use elsewhere. Research pending; his call.
