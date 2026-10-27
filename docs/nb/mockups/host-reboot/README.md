# Host reboot design review — revision 02

Open `index.html` directly in a browser. Keep `assets/` beside it; all images,
styles, and scripts are local, with no application or web server required.

## Contents

- Current host list and host detail screenshots.
- A permanent, normal-sized Reboot header action, enabled even without a pending
  reboot (subject to connection, permissions, capability, and operation guards).
- Contextual notices only when installed updates require a restart; stronger
  emphasis for confirmed security-related pending restarts. No second action in
  the banner.
- Manual reboot confirmation, offline, and mobile states.
- Full-size image viewer and a No restart / Other updates / Security switch.

## Capture provenance

- Captured using `playwright-cli` on 27 October 2026.
- Actual UI from the local `patchmon-nb:smoke` image, version 2.1.6, built from
  source at `d6e91df2` plus the image-name changes (see `../../docker-images.md`).
- The isolated `patchmon-nb-smoke` stack was seeded with four **synthetic** hosts
  and eight demo packages. IP addresses use the documentation-only `192.0.2.0/24`
  range. No agent was installed, connected, or rebooted.
- The two `current-*` screenshots show the unmodified application UI and its real
  offline connection state, using that synthetic dataset.
- Concepts were made by inserting temporary DOM/CSS into the page and simulating
  connected status in browser request interception. Healthy/offline states and
  the confirmation are illustrations, not proof of working reboot behaviour.
- Desktop captures: 1536 × 1100. Mobile capture: 390 × 1000. Existing dark theme.
- Revision 02 uses the existing toolbar button's actual classes, font size,
  weight, and 38px height. Browser checks verified equal button heights, enabled
  manual reboots without a pending flag, disabled offline controls, and no
  horizontal overflow in either capture size.
- Security/update reasons in the new screenshots are simulated. The eventual
  feature must establish a trustworthy source for security classification and
  fall back to a generic reason when that classification is unknown.
- No production frontend, server, agent, or database schema code was changed.

This review does not implement the feature. Permission checks, agent capability,
operation guards, request outcomes, and audit behaviour still need implementation
and verification after the design is agreed.
