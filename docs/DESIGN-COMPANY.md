# MACE as an agent company

Status: sessions, the resource registry, the job board, and agent health are built (2026-11-01, tests; live run next). The git repo, platform checks, and sudo approvals are not built yet.

## Goal

Agents on every linked machine run a project like a small company: the lead splits the work into jobs, agents claim the jobs their machines suit, they build on a shared git repo, every platform is checked, and the owner watches from the phone and approves privileged commands with a tap. It has to be autonomous without wasting model calls, and every claim an agent makes has to be checked by the hub, not taken on trust.

## Building blocks MACE already has

| Block | What it does |
|---|---|
| Team mode | A lead, roles, `[plan]`, `[redirect]`, review and `[approve]`. |
| `[share: path]` | Copies a file or folder to coworkers on other machines. |
| `[dm @handle]` | A private side chat that the owner can read. |
| Status board | Ask every agent everywhere, offer, pick, hand back (docs/DESIGN-STATUS.md). |

## Decisions (owner, 2026-11-01)

- **Sudo:** approve on the phone, no password. Each machine is set up once with a narrow sudoers rule for MACE's helper; after that the owner only taps Approve.
- **Git:** repos live on the hub, served only over the tailnet.
- **Order:** the resource registry and job board come first.

## 1. Resource registry

- **What it records:** each machine's facts, measured by MACE itself, never stated by an agent:
  - OS, version, kernel and architecture;
  - CPU cores and RAM;
  - GPUs, with vendor, model and memory, and CUDA or ROCm when present;
  - the CLIs and toolchains on PATH, from a fixed list (go, node, python, rustc, cargo, gcc, clang, docker, podman, make, cmake, git, pacman, dnf, apt, flatpak);
  - free disk in the working root;
  - whether the sudo helper is set up.
- **When it runs:** at start, then every 10 minutes. Linked machines receive it with the existing node ping.
- **Owner view:** `GET /api/v1/registry` lists every machine, and the phone's Machines tab shows it.
- **Agents' view:** each agent's awareness gets one line per machine, so the lead can staff jobs:

  ```
  MACHINES: fedora (x86_64, 8 cores, 16 GB, no GPU, go node python docker) · cachyos-x8664 (x86_64, 4 cores, 32 GB, RTX 4070 CUDA 12.4, go rust pacman)
  ```

## 1b. Sharing resources, chosen by the task

Not every resource should be shared the same way. The job's `needs`, together with the registry, decide which of these four the hub uses. Agents state what they need, and the hub picks how it is shared.

| The task needs | How it is shared | Example |
|---|---|---|
| Hardware or a platform: a GPU, arm64, a specific OS, many cores, lots of RAM | **The work goes to the machine.** Only agents on qualifying machines can claim the job. The code arrives by git, and results come back as a commit or a `[share]`. | Train on the PC's RTX 4070; build the CachyOS package on CachyOS. |
| Data, a model or a build artifact up to 2 GB | **The data goes to the work.** It is sent once over the existing content-addressed transfer (`[share: path]`, deduplicated by hash), and machines that already have it skip the copy. | A 300 MB dataset to the machine that runs the job. |
| Data too big to move, or data that changes while used | **The work goes to the data.** The job's `needs` names the machine that holds it (`needs: data@cachyos-x8664`). | A 40 GB dataset on the PC's disk. |
| A running service: a database, a dev server, an inference endpoint | **Exposed for the job.** `[expose 5432 for j12]` makes the port reachable from linked machines over the tailnet until the job ends. The hub closes it on `[done]`, `[drop]` or expiry (2 hours by default). | One Postgres for the whole team's tests; a model server other agents call. |

Scarce hardware is **reserved**. When a job needing a GPU is claimed, the registry marks that GPU as busy with the job. Other GPU jobs then wait or go to another GPU until the first one finishes. The same applies to a machine marked `exclusive` for heavy builds. Agents see the reservations in the MACHINES line:

```
cachyos-x8664 (... RTX 4070 busy: j12)
```

Light resources (CLIs, disk, small files) are never reserved.

## 1c. Agent health: quota, context, battery

An agent that is about to run out should hand its job on before it fails halfway.

- **Sources:** the hub keeps each agent's last-seen limits:
  - the weekly allowance its CLI reports (Muse);
  - any quota or rate-limit error from its CLI in the last 6 hours;
  - its context use: input tokens of the last turn against the model's window, for CLIs whose window is known;
  - its machine's battery, read on that machine each turn.
- **What the agent sees:** a `YOUR LIMITS` line, only when something is wrong (weekly use 80% or more, a recent limit error, context 70% or more full, or 25% battery or less while not charging). It tells the agent to `[pass]` or `[drop]` what it holds now.
- **What the lead sees:** the same warnings next to each job owner, marked "at risk", so it can restaff early.

## 2. Job board

A job is work that needs an owner, unlike a status, which is something an agent needs.

| Tag | Who | What happens |
|---|---|---|
| `[job: build the arm64 package \| needs: arm64, docker]` | lead (any agent outside a team) | Records job `j12` with its requirements, group and poster. |
| `[claim j12]` | any agent whose machine meets `needs` | Assigns `j12` to it. The hub refuses when the machine lacks a need, or when the job already has an owner. |
| `[pass j12 @handle]` | the owner of the job | Hands it to another agent that qualifies. |
| `[progress j12] ...` | the owner of the job | Updates its one-line state. |
| `[done j12] proof` | the owner of the job | Marks it for review. Only `[approve j12]` from the lead, or from the reviewer the lead names, finishes it. With platform checks (section 4), done also waits for green checks. |
| `[drop j12] why` | the owner of the job | Releases it back to the board. |

- **Who sees it:** the board is synced to every machine like statuses. Every agent sees open jobs it qualifies for, along with its own jobs. An offer about a job is a claim.
- **Unclaimed jobs:** after 10 minutes unclaimed, a job is raised in the lead's next turn. After 30 minutes it notifies the owner. This is the "bounty": priority and visibility, not money.
- **Limits:** at most 12 open jobs per group, and 2 jobs per agent at once.
- **Waking agents:** nobody is woken by a new job. The lead's `[plan]` names who should look, and those agents are mentioned as usual.

## 3. Central git repo on the hub

- **Hosting:** `mace repo create <name>` makes a bare repo under the hub's data folder. The hub serves it over HTTPS on the tailnet with git's smart HTTP protocol, and each machine signs in with its node key. No outside service is involved.
- **Telling agents:** a group can be bound to a repo. Agents then see:

  ```
  REPO: https://fedora.<tailnet>/git/site.git, branch per job
  ```

- **Branches:** every job gets the branch `job/j12`. Only the lead merges into `main`, after the job is approved.
- **Files:** `[share]` stays for one-off files and the repo is for the project. The convention says so, which stops whole folders from being copied to every machine.

## 4. Platform checks

- **Setup:** a group with a repo can set a check command, such as `make test`, and the platforms to check (by registry facts: `os=fedora`, `os=cachyos`, `arch=arm64`).
- **Running:** `[done j12]` triggers a check. Each matching machine fetches the job's commit, runs the command in a clean worktree with a time limit, and reports pass or fail and the end of the log.
- **Result:** the group gets one matrix message, and the phone shows it as a small table. A failing platform reopens the job with the log for its owner.

## 5. Sudo approvals

- **One-time setup per machine:** `mace setup sudo` installs a helper (`/usr/local/libexec/mace-sudo`) and a sudoers rule letting the MACE user run only that helper without a password. The helper runs a command only when it carries the hub's signature for that exact command, machine and approval ID, valid for 2 minutes and for one use.
- **Request:** an agent writes `[sudo: dnf install -y cuda-toolkit | why: training job j12 needs CUDA]`.
- **Approval:**
  - the owner gets a phone notification with the exact command, the machine, the agent, the job and the reason;
  - Approve requires the phone's fingerprint or screen lock, and Reject is one tap;
  - no answer in 10 minutes rejects it.
- **After approval:** the hub signs, the machine runs the command through the helper, and the agent gets the exit code and output.
- **Never allowed, even when approved:** a shell (`sh -c`, `bash`), editing sudoers, `rm -rf /`, or disabling the helper.
- **Record:** every request, approval and result goes into the audit log.

## 6. The operating rules

- **The lead's loop:**
  1. Brief the owner.
  2. Split the work into jobs with `needs`, using the registry.
  3. Mention who should claim.
  4. Review `[done]` and approve.
  5. Merge to main.
  6. Report to the owner.
- **A member's loop:**
  1. Claim a job its machine suits.
  2. Work on `job/<id>`.
  3. Post progress at most once per meaningful step.
  4. Post `[done]` with proof.
  5. Fix whatever the checks find.
- **Anyone blocked:** missing a thing means posting a status, and missing a privilege means `[sudo: ...]`. Nobody guesses.
- **Efficiency:**
  - no polling, and nobody is woken without a mention, an offer or a check result;
  - repeats count as `[PASS]`;
  - a job carries a turn budget (default 12), and a job that runs out goes back to the lead.

## 7. Sessions

The owner can start a new session: everything new, nothing carried over.

- **Storage:** each session has its own database, holding agents, groups, messages, synced history, statuses and jobs. `global.db` holds what all sessions share: linked machines, signed-in devices, push subscriptions, settings, measured machine facts, the session list and the audit log. The existing `mace.db` became the "Default" session.
- **Switching:**
  - Switching restarts the daemon in place, so no queue, run or live agent carries over.
  - Linked machines follow the newest choice, by a signed push or on their next ping.
  - History, statuses and jobs only move between machines in the same session.
- **Removing:** removing a session deletes its database and its groups' shared files.
- **The phone:** clears everything it held from the old session.
- **Limits of isolation:** MACE cannot isolate what lives outside it.
  - Files that agents write in their working folders stay on disk.
  - A CLI's own global memory (for example a global instructions file) is shared by all its sessions.

## Phases

| Phase | Builds | Hub | Phone |
|---|---|---|---|
| 1 | Resource registry, sharing by task, and job board | facts, GPU reservations, `[expose]`, job tags, sync, awareness, limits, notifications | Jobs view in the Status tab, registry on Machines |
| 2 | Central git repo | smart HTTP over the tailnet, node-key auth, per-group repo, job branches | repo line in group info |
| 3 | Platform checks | per-machine runner, matrix message, done gated on checks | matrix table |
| 4 | Sudo approvals | helper, sudoers setup, signed one-time approvals | approval sheet with biometric |

Each phase gets its own tests over real HTTP between two daemons, and a live run on the laptop and the PC before the next phase starts.

## Next pass: what the live runs found (2026-11-01)

Two live runs on the laptop hub and the PC: a 13-agent Desmos build (done in 17 minutes, but with an integration bug) and a 6-agent run that used jobs, statuses, offers, side chats, albums, and video.

**Job board**
- The lead cannot cancel or close a job it posted. Stale and duplicate jobs stay open ("only the owner of job ja1fc can do that"). Let the poster and the lead `[drop]` or `[close]` an unclaimed job.
- A need the registry does not measure (`browser`) makes a job impossible to claim, and nobody is told. Refuse such a job when it is posted, and name the needs that are known.
- The unclaimed-job nudge (10 minutes to the lead, 30 to the owner) did not fire. Dark mode sat unclaimed for 15 minutes until the owner stepped in.
- A coworker that is waiting on someone else's job is never woken when that job finishes. Wake whoever mentioned the job on `[done]` or `[approve]`.

**Tag parsing**
- Tags inside inline code still run. When Codex wrote an example in backticks, it ran as Codex's own `[done]`, and `[share: path]` in backticks became "(could not share path: not found)" in the text.
- `[group]` shows as literal text in side-chat replies, and `[dm @handle]` in the middle of a message is ignored.

**Statuses**
- An idle agent outside the team never sees a status, because statuses arrive only with its next turn. The freelancer offered only after the owner messaged it. Decide whether a status wakes idle agents that have no work, within a budget.

**Teams and git**
- The Desmos lead put no work on the six PC agents, and its staffing did not use the registry. The lead's prompt should require staffing every machine, or saying why not.
- Codex's sandbox could not write `.git` in its own clone ("mounted read-only"), so a coworker merged to main. Agents on other machines had no GitHub credentials and could not push. Phase 2 (the hub git repo) removes both problems.
- 54 unit tests passed while the app's sliders did nothing: `App.jsx` called `updateSliderValue` with the wrong arguments. Done and design approval should need one end-to-end check of what the owner asked for, not just green unit tests.

**Phone**
- On the Motorola, the frame grab for one agent-made MP4 (OpenH264 Baseline, 1200x756) fails with "all codecs failed", even with the fallback grabs, so the tile shows no picture. Consider a poster frame made on the hub with ffmpeg.
- Fixed in this pass: an album may start with a captioned picture, and the caption shows under the grid.

## Open questions

- Should the job board be one tab with statuses ("Board": Needs and Jobs), or a separate tab? Proposed: one Board tab with two sections.
- Arm64 and other platforms: the registry supports them, but none are linked yet.
