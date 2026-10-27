# Patch Selected · design review 1

Open `index.html` locally, or serve `docs/nb/mockups/` with a static HTTP server.
No external assets, dependencies, API calls, or real host actions.

Try changing row checkboxes, opening **Patch Selected**, choosing either mode,
reviewing the skipped-host reasons, and submitting a simulated result. The
Timing/Approval values are illustrative; implementation must use the real wizard.
Resize to 390px for the mobile layout.

The ready/blocked outcomes and package counts are synthetic. They are not a
claim of OS support or working security classification. The Arch example is
deliberately blocked for security-only mode.

## Captures

- [Desktop menu](menu-desktop.png)
- [Desktop security review](security-review-desktop.png)
- [Mobile menu](menu-mobile.png)
- [Mobile security review](security-review-mobile.png)

Playwright checked both mode summaries, explicit Arch exclusion, confirmation
gating, cancel, simulated submission, empty-selection disabling, and no page
overflow at 390px. These checks verify the mockup only, not application code.
