import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
const { chromium } = await import(process.env.MACE_PLAYWRIGHT || "playwright");
const phase = process.argv[2] || "before",
  root = process.cwd(),
  out = join(root, ".artifacts/ux-review", phase),
  state = mkdtempSync(join(tmpdir(), "mace-ux-"));
mkdirSync(out, { recursive: true });
const env = {
  ...process.env,
  MACE_HOME: state,
  MACE_AUTH_SECRET: "",
  MACE_TEST_MODE: "1",
  MACE_PID_FILE: join(state, "maced.pid"),
  MACE_LOG_FILE: join(state, "maced.log"),
};
let browser;
const report = { phase, views: [], typing: {}, errors: [] };
try {
  const start = spawnSync(
    join(root, "bin/mace"),
    [
      "start",
      "--host",
      "127.0.0.1",
      "--port",
      "0",
      "--db",
      join(state, "mace.db"),
    ],
    { env, encoding: "utf8" },
  );
  assert.equal(start.status, 0, start.stderr);
  const runtime = JSON.parse(readFileSync(join(state, "runtime.json"), "utf8")),
    token = readFileSync(join(state, "active-token"), "utf8");
  const api = async (path, body) => {
    const r = await fetch(runtime.url + "/api/v1" + path, {
      method: body === undefined ? "GET" : "POST",
      headers: {
        Authorization: "Bearer " + token,
        "Content-Type": "application/json",
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    if (!r.ok) throw Error(await r.text());
    return r.json();
  };
  const a = await api("/sessions", {
      provider: "diagnostic",
      name: "Code review (test)",
      working_dir: root,
    }),
    b = await api("/sessions", {
      provider: "diagnostic",
      name: "Research and documentation review (test)",
      working_dir: "/tmp",
    });
  const room = await api("/rooms", {
    name: "Mobile release review (test)",
    members: [a.id, b.id],
  });
  const idle = async (id) => {
    for (let i = 0; i < 80; i++) {
      const r = (await api("/runs")).find((r) => r.id === id);
      if (r && r.status !== "running") return;
      await new Promise((r) => setTimeout(r, 50));
    }
  };
  await api("/chats/" + a.id + "/messages", {
    text: "Please review the mobile layout and check the composer, the folder picker, and the conversation controls.",
  });
  await idle(a.id);
  await api("/chats/" + a.id + "/messages", {
    sender: a.id,
    text: '## Review notes\n\nThe draft should stay in place while you switch conversations.\n\n- Keep the Send button reachable.\n- Let longer messages wrap naturally.\n- Preserve a comfortable reading size.\n\n```ts\nconst draft = "A multiline message";\nsendMessage(draft);\n```\n\nThis is diagnostic fixture content for the layout review, not a live model assessment.',
  });
  await idle(a.id);
  await api("/chats/" + room.id + "/messages", {
    text: "This group is a diagnostic fixture for the mobile control review.",
  });
  await idle(room.id);
  browser = await chromium.launch({
    executablePath: process.env.MACE_BROWSER,
    headless: true,
    args: ["--no-sandbox"],
  });
  async function capture(page, label) {
    await page.screenshot({ path: join(out, label + ".png") });
    const metrics = await page.evaluate(() => {
      const visible = (e) => {
        const r = e.getBoundingClientRect(),
          s = getComputedStyle(e);
        return (
          r.width > 0 &&
          r.height > 0 &&
          s.visibility !== "hidden" &&
          r.bottom > 0 &&
          r.top < innerHeight
        );
      };
      const scope=document.querySelector('dialog[open]')||document;
      const rgb=value=>(value.match(/[\d.]+/g)||[]).map(Number);
      const background=element=>{for(let e=element;e;e=e.parentElement){const c=rgb(getComputedStyle(e).backgroundColor);if(c.length===3||c[3]===1)return c.slice(0,3)}return [255,255,255]};
      const luminance=color=>color.map(v=>{v/=255;return v<=.04045?v/12.92:((v+.055)/1.055)**2.4}).reduce((sum,v,i)=>sum+v*[.2126,.7152,.0722][i],0);
      const contrast=element=>{const a=luminance(rgb(getComputedStyle(element).color).slice(0,3)),b=luminance(background(element));return Math.round((Math.max(a,b)+.05)/(Math.min(a,b)+.05)*100)/100};
      return {
        width: innerWidth,
        height: innerHeight,
        overflow: document.documentElement.scrollWidth > innerWidth,
        appWidth: document.querySelector(".app")?.getBoundingClientRect().width,
        smallTargets: [
          ...scope.querySelectorAll("button,a,input[type=checkbox]"),
        ]
          .filter(visible)
          .map((e) => {
            const target=e.matches('input[type=checkbox]')?(e.closest('label')||e):e;
            const r = target.getBoundingClientRect();
            return {
              name:
                e.getAttribute("aria-label") ||
                e.textContent.trim().slice(0, 45),
              width: Math.round(r.width),
              height: Math.round(r.height),
            };
          })
          .filter((e) => e.width < 44 || e.height < 44),
        text: [
          ...document.querySelectorAll(
            ".chatcopy p,.threadtitle small,.policy-strip,.message-text,.composer textarea,.page h1",
          ),
        ]
          .filter(visible)
          .map((e) => ({
            name: e.className || e.tagName,
            size: getComputedStyle(e).fontSize,
            color: getComputedStyle(e).color,
            background: getComputedStyle(e).backgroundColor,
            contrast:contrast(e),
          })),
        dialogOverflow: document.querySelector("dialog[open]")
          ? document.querySelector("dialog[open]").scrollWidth >
            document.querySelector("dialog[open]").clientWidth
          : false,
      };
    });
    report.views.push({ label, ...metrics });
    writeFileSync(join(out, "report.json"), JSON.stringify(report, null, 2));
  }
  if(phase==='after'){
    const emptyState=join(state,'empty');mkdirSync(emptyState);const emptyEnv={...env,MACE_HOME:emptyState,MACE_PID_FILE:join(emptyState,'maced.pid'),MACE_LOG_FILE:join(emptyState,'maced.log')};
    const result=spawnSync(join(root,'bin/mace'),['start','--host','127.0.0.1','--port','0','--db',join(emptyState,'mace.db')],{env:emptyEnv,encoding:'utf8'});assert.equal(result.status,0,result.stderr);
    const emptyRuntime=JSON.parse(readFileSync(join(emptyState,'runtime.json'),'utf8')),emptyToken=readFileSync(join(emptyState,'active-token'),'utf8');const context=await browser.newContext({viewport:{width:390,height:844},deviceScaleFactor:2,isMobile:true,hasTouch:true});
    try{const page=await context.newPage();await page.goto(emptyRuntime.url+'/#connect='+emptyToken);await page.getByRole('button',{name:'Choose an agent',exact:true}).waitFor();await capture(page,'phone-empty-chats');await page.getByRole('button',{name:'Activity',exact:true}).click();await page.getByRole('heading',{name:'Nothing has run yet.'}).waitFor();await capture(page,'phone-empty-activity');await page.setViewportSize({width:1440,height:960});await capture(page,'desktop-empty-activity');await page.getByRole('button',{name:'More',exact:true}).click();await page.getByRole('button',{name:'Apps',exact:true}).click();await page.waitForSelector('.apps-list button');await capture(page,'desktop-apps');const appNames=await page.locator('.apps-list>button>span').allTextContents();report.typing.appsAlphabetical=appNames.every((name,i)=>i===0||appNames[i-1].localeCompare(name)<=0);await page.setViewportSize({width:390,height:844});await capture(page,'phone-apps');await page.getByRole('textbox',{name:'Search host apps'}).fill('no-such-application');await capture(page,'phone-apps-no-results')}
    finally{await context.close();spawnSync(join(root,'bin/mace'),['stop'],{env:emptyEnv,encoding:'utf8',timeout:15000})}
  }
  for (const config of [
    { label: "phone", width: 390, height: 844, dpr: 3, touch: true },
    { label: "small-phone", width: 320, height: 640, dpr: 2, touch: true },
    { label: "tablet-700", width: 700, height: 900, dpr: 2, touch: true },
    { label: "desktop", width: 1440, height: 960, dpr: 1, touch: false },
    { label: "short-laptop", width: 1280, height: 640, dpr: 1, touch: false },
  ]) {
    const context = await browser.newContext({
      viewport: { width: config.width, height: config.height },
      deviceScaleFactor: config.dpr,
      isMobile: config.touch,
      hasTouch: config.touch,
    });
    const page = await context.newPage();
    page.on("pageerror", (e) => report.errors.push(e.message));
    page.setDefaultTimeout(7000);
    try {
      await page.goto(runtime.url + "/#connect=" + token);
      await page
        .getByRole("button", { name: "New agent", exact: true })
        .waitFor();
      await capture(page, config.label + "-chats");
      await page
        .getByRole("button", { name: /Code review \(test\)/ })
        .first()
        .click();
      await page.waitForFunction(
        () => document.querySelectorAll(".message-text").length >= 3,
      );
      if(phase==='after')await page.waitForSelector('.message-text.markdown h2');
      await capture(page, config.label + "-thread");
      const input = page.getByRole("textbox", { name: "Message", exact: true });
      await input.fill("First line\nSecond line\nThird line\nFourth line");
      report.typing[config.label + "-multiline-height"] = await input.evaluate(
        (e) => e.getBoundingClientRect().height,
      );
      await capture(page, config.label + "-multiline");
      if (config.label === "phone") {
        await input.fill("IME composition must not send this draft");
        await input.evaluate((e) =>
          e.dispatchEvent(
            new KeyboardEvent("keydown", {
              key: "Enter",
              isComposing: true,
              bubbles: true,
            }),
          ),
        );
        await page.waitForTimeout(650);
        report.typing.imePreserved = (await input.inputValue()).includes(
          "IME composition",
        );
        await page.waitForTimeout(3000);
        await input.fill("Mobile Enter should insert a new line");
        await input.press("Enter");
        await page.waitForTimeout(650);
        report.typing.mobileEnterPreserved = (
          await input.inputValue()
        ).includes("Mobile Enter");
        await input.fill("A draft kept safely while the network is offline");
        await context.setOffline(true);
        await page.waitForTimeout(3800);
        report.typing.offlineWorkspaceVisible = await page
          .locator(".thread")
          .isVisible();
        await capture(page, "phone-offline");
        await context.setOffline(false);
        await page.waitForTimeout(3800);
        if (await page.getByRole("button", { name: "Dismiss error" }).count())
          await page.getByRole("button", { name: "Dismiss error" }).click();
        await page
          .getByRole("button", { name: "Conversation controls" })
          .click();
        await capture(page, "phone-controls");
        let focusContained=true;for(let i=0;i<10;i++){await page.keyboard.press('Tab');focusContained&&=await page.evaluate(()=>document.querySelector('dialog[open]')?.contains(document.activeElement))}report.typing.dialogFocusContained=focusContained;await page.keyboard.press('Escape');
        await page.getByRole("button", { name: "Attach host file" }).click();
        await capture(page, "phone-files");
        await page.getByRole("button", { name: "Close dialog" }).click();
        await page.setViewportSize({ width: 390, height: 484 });
        await input.focus();
        await capture(page, "phone-keyboard-height");
        await page.setViewportSize({ width: 390, height: 844 });
        await page.getByRole("button", { name: "Back to chats" }).click();
        await page
          .getByRole("button", { name: "New group", exact: true })
          .click();
        await page.getByLabel("Group name").fill("No members selected");
        if (
          await page
            .getByRole("button", { name: "Create group", exact: true })
            .isEnabled()
        ) {
          await page
            .getByRole("button", { name: "Create group", exact: true })
            .click();
          await page.waitForTimeout(400);
        } else {
          report.typing.emptyGroupPrevented = true;
        }
        report.typing.dialogErrors = await page
          .locator("dialog [role=alert]")
          .count();
        await capture(page, "phone-group-error");
        await page.getByRole("button", { name: "Close dialog" }).click();
        await page.getByRole('button',{name:'More',exact:true}).click();await page.getByRole('button',{name:'Connected hosts',exact:true}).click();await page.getByRole('button',{name:'Connect a host',exact:true}).click();await page.getByLabel('Host address').fill('http://127.0.0.1:1');await page.getByLabel('Access token',{exact:true}).fill('diagnostic-invalid-token');await page.getByRole('button',{name:'Connect host',exact:true}).click();await page.waitForTimeout(400);report.typing.peerDialogErrors=await page.locator('dialog [role=alert]').count();await capture(page,'phone-peer-error');await page.getByRole('button',{name:'Close dialog'}).click();await page.getByRole('button',{name:'Chats',exact:true}).click();
        await page
          .getByRole("button", { name: /Mobile release review/ })
          .click();
        await page
          .getByRole("button", { name: "Conversation controls" })
          .click();
        await capture(page, "phone-group-controls");
        await page.getByRole("button", { name: "Close dialog" }).click();
        await page
          .getByRole("button", { name: "Settings", exact: true })
          .click();
        await capture(page, "phone-settings");
        await page.getByRole("button", { name: "Switch to dark" }).click();
        await capture(page, "phone-settings-dark");
        await page.getByRole("button", { name: "Chats", exact: true }).click();
        await capture(page, "phone-thread-dark");
      }
      if (config.label === "desktop") {
        await page.evaluate(
          () => (document.documentElement.style.fontSize = "200%"),
        );
        await capture(page, "desktop-text-200");
        await page.evaluate(
          () => (document.documentElement.style.fontSize = ""),
        );
        await page.setViewportSize({ width: 640, height: 450 });
        await capture(page, "desktop-200-percent-equivalent");
      }
      await page.getByRole("button", { name: "Host", exact: true }).click();
      await capture(page, config.label + "-host");
      await page.getByRole("button", { name: "Activity", exact: true }).click();
      await page.waitForSelector(".audit-list details");
      await capture(page, config.label + "-activity");
    } catch (e) {
      report.errors.push(config.label + ": " + e.message.split("\n")[0]);
      await capture(page, config.label + "-failure");
    } finally {
      await context.close();
    }
  }
  writeFileSync(join(out, "report.json"), JSON.stringify(report, null, 2));
  console.log("Captured", report.views.length, "views:", out);
  console.log("Typing:", JSON.stringify(report.typing));
  console.log(
    "Overflow:",
    report.views
      .filter((v) => v.overflow || v.dialogOverflow)
      .map((v) => v.label)
      .join(", ") || "none",
  );
} finally {
  if (browser) await browser.close();
  spawnSync(join(root, "bin/mace"), ["stop"], {
    env,
    encoding: "utf8",
    timeout: 15000,
  });
}
