# MACE checkpoint

Saved at the owner's request on 2026-10-31. Further testing and polish stopped to preserve quota.

## Included in this commit

- Group removal, member removal, optional member roles, and continuous discussion without a typed starting prompt.
- Automatic MACE identity, coworker instructions, roster, role, and conversation context passed to native agents on every turn.
- Cancelling or removing one member lets remaining group members continue.
- Mobile workspace spacing, quieter application lists, useful empty states, readable agent Markdown, multiline composition, IME handling, drafts, and offline workspace preservation.
- Local listener discovery with `mace ports`, `mace status --port NUMBER`, and targeted `mace stop --port NUMBER` or `mace kill --port NUMBER`. Targeted commands verify that the listener belongs to Mace.

## Verification already completed

- Backend tests, native prompt/context tests, and API/agent race tests passed during implementation.
- Go binaries and the production frontend built successfully during this pass.
- A browser review captured 43 views across phone, tablet, desktop, dark theme, larger text, offline, and dialog/error states. Evidence is local under `.artifacts/ux-review/after/` and is intentionally ignored by Git.
- That review verified multiline growth, IME composition, mobile Enter behavior, offline workspace preservation, alphabetical apps, and prevention of empty groups. It found dialog focus containment and selected-row text contrast issues; fixes were applied afterward but their browser checks have not been rerun.
- Real agy and muse agents exchanged three autonomous replies with no owner message and no assigned roles. Their transcript and phone screenshot exist under `.artifacts/ux-review/live-team/`. The saved transcript proves participation; final cleanup/group-removal test output was not recovered before this checkpoint.
- `git diff --check` passed immediately before saving this checkpoint.

## Remaining verification

1. Update `scripts/check-browser.mjs` for the new `Save and start` continuous-mode action and capitalized run labels. Exercise role editing and group removal.
2. Rerun `scripts/review-workspace.mjs` after the final focus/contrast changes and inspect the new member-role dialog on narrow phones.
3. Recover or rerun the latest CLI port lifecycle test result. Do not treat the latest port-specific changes as fully verified yet.
4. Run the Impeccable detector once on changed frontend files and review actionable findings.
5. Run final build/tests after any further edits. Refresh installed binaries and web assets from `bin/` and `web/dist/`, then restart the user's server when ready. The user-local installation is not confirmed to contain this checkpoint.

## Free old listening ports

Ports are freed by stopping their listening server. With the CLI built from this checkpoint:

```sh
bin/mace ports
bin/mace stop --port 8080
# Only if graceful stopping fails:
bin/mace kill --port 8080
```

Replace 8080 with a port shown by `ports`. These commands refuse unrelated listeners. Use `mace` instead of `bin/mace` after refreshing the installed CLI. Do not remove databases or tokens to free a port.

The earlier baseline is commit `6fc802b`. This checkpoint deliberately records remaining work rather than claiming the final review is complete.
