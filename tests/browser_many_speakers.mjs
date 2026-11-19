// Build web/dist first. Run with node tests/browser_many_speakers.mjs [screenshot-directory].
// Use AIR2MI_NATIVE=1 for the Android build, and CHROMIUM_PATH for a local browser.
import assert from 'node:assert/strict';
import { mkdir } from 'node:fs/promises';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium } from '../web/node_modules/playwright-core/index.mjs';
import { preview } from '../web/node_modules/vite/dist/node/index.js';

const screenshots = process.argv[2];
const native = process.env.AIR2MI_NATIVE === '1';
const rooms = ['客厅', '书房', '主卧', '餐厅', '厨房', '阳台', '影音室', '客房'];
function snapshot(count) {
  const speakers = Array.from({ length: count }, (_, i) => {
    const room = i === 6 ? '二楼西侧影音室家庭影院环绕声播放设备' :
      i === 3 ? 'DiningRoomWirelessSpeakerLongDeviceName' : rooms[i % rooms.length];
    return { speaker_id: 's' + i, display_name: room + ' · ' + (i + 1),
      enabled: true, ip: '192.0.2.' + (i + 1) };
  });
  const inputs = speakers.map((s, i) => ({ input_id: 'i' + i, enabled: true,
    display_name: s.display_name + ' AirPlay', options: { speaker_id: s.speaker_id } }));
  return {
    app_version: 'preview', engine_installed: true, engine_running: true,
    engine_manifest: { kernel_version: 'preview' }, inputs, speakers,
    state: {
      instances: Object.fromEntries(inputs.map(i => [i.input_id, { selected: true }])),
      river: { producer: inputs.at(-1).input_id, references: inputs.slice(0,-1).map(i => i.input_id) },
      speakers: speakers.map(s => ({ ...s, selected: true, playback_state: 'playing' })),
    },
  };
}

async function fits(page) {
  assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'horizontal overflow');
}

async function checkDotMap(page, inputs, speakers) {
  const map = page.locator('.compact-flow');
  const caption = map.locator('figcaption');
  assert.equal(await caption.locator('strong').count(), 0, 'device name visible before selection');
  assert.equal(await map.locator('button:has-text("AirPlay")').count(), 0);
  for (const [label, names] of [['AirPlay 接收器', inputs], ['妙播音箱', speakers]]) {
    const bank = map.getByRole('group', { name: label + '光点' });
    const dots = bank.locator('button');
    assert.equal(await dots.count(), names.length);
    assert.equal(await bank.locator('path').count(), names.length);
    const hub = await map.locator('.map-hub').boundingBox();
    const positions = await dots.evaluateAll(buttons => buttons.map(button => {
      const rect = button.getBoundingClientRect();
      return { x: rect.x + rect.width / 2, y: rect.y + rect.height / 2 };
    }));
    positions.forEach((dot, index) => {
      assert(Math.abs(dot.x - positions[0].x) < 1, 'nodes must share a column');
      assert(index === 0 || dot.y > positions[index - 1].y, 'nodes must run top to bottom');
      assert(label === '妙播音箱' ? dot.x > hub.x + hub.width : dot.x < hub.x, 'wrong side of hub');
    });
    const centerY = (positions[0].y + positions.at(-1).y) / 2;
    assert(Math.abs(centerY - (hub.y + hub.height / 2)) <= 1, `hub not vertically centered: ${label}, nodes ${centerY}, hub ${hub.y + hub.height / 2}`);
    const colors = await dots.locator('i').evaluateAll(items => items.map(i => getComputedStyle(i).backgroundColor));
    assert.deepEqual([...new Set(colors)], ['rgb(75, 152, 207)']);
    assert(await dots.evaluateAll(buttons => {
      const bounds = buttons.map(button => button.getBoundingClientRect());
      return bounds.every((r, i) => r.width >= 24 && r.height >= 24 &&
        bounds.slice(i + 1).every(s => r.right <= s.left + 0.1 || s.right <= r.left + 0.1 ||
          r.bottom <= s.top || s.bottom <= r.top));
    }), 'overlapping or undersized touch targets');
    for (const index of [0, Math.floor(names.length / 2), names.length - 1]) {
      await dots.nth(index).click();
      assert.equal(await caption.locator('strong').innerText(), names[index]);
      if (names.length === 24) {
        const detail = await caption.boundingBox();
        const point = await dots.nth(index).boundingBox();
        assert(Math.abs((detail.y + detail.height / 2) - (point.y + point.height / 2)) < 2,
          'name should be beside the selected dot in a tall diagram');
      }
      assert.equal(await map.locator('.map-point[aria-pressed=true]').count(), 1);
    }
    await dots.last().click();
    assert.equal(await caption.locator('strong').count(), 0);
  }
}

async function checkWideGraph(page, names) {
  const graph = page.locator('.topology-wide');
  assert.deepEqual(await graph.locator('.svg-node title').allTextContents(), names);
  assert.equal(await graph.locator('.stream-edge').count(), names.length);
  assert(await graph.evaluate(svg => {
    const extent = svg.getBoundingClientRect();
    const rectangles = [...svg.querySelectorAll('.svg-node > rect:first-of-type')]
      .map(node => node.getBoundingClientRect());
    return rectangles.every((r, i) => r.top >= extent.top && r.bottom <= extent.bottom &&
      r.left >= extent.left && r.right <= extent.right &&
      rectangles.slice(i + 1).every(s => r.right <= s.left || s.right <= r.left ||
        r.bottom <= s.top || s.bottom <= r.top));
  }), 'clipped or overlapping desktop nodes');
  assert(await graph.locator('.svg-node').evaluateAll(nodes => nodes.every(node => {
    const name = node.querySelector('.diagram-name, .svg-name');
    const dot = node.querySelector('.svg-dot').getBoundingClientRect();
    return name.getBoundingClientRect().right < dot.left &&
      (name.scrollWidth <= name.clientWidth || getComputedStyle(name).textOverflow === 'ellipsis');
  })), 'desktop name covers status dot');
}

async function checkList(page, url, path, selector, names) {
  await page.goto(url + '/#/' + path);
  const rows = page.locator(selector);
  await rows.last().waitFor();
  assert.deepEqual(await rows.locator('h3').allTextContents(), names);
  await fits(page);
  // Scroll as a user would; the fixed mobile navigation must leave the last action reachable.
  await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight));
  const edit = rows.last().getByRole('button', { name: '编辑 ' + names.at(-1), exact: true });
  assert(await edit.evaluate(button => {
    const r = button.getBoundingClientRect();
    return button.contains(document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2));
  }), 'last edit button obscured at end of list');
  await edit.click();
  const dialog = page.getByRole('dialog');
  assert.equal(await dialog.getByLabel('名称', { exact: true }).inputValue(), names.at(-1));
  await page.keyboard.press('Escape');
}

// Resize the already-mounted application. This catches device detection and
// mount-time width checks that separate fixed-viewport page loads cannot catch.
async function checkLiveResize(page, speakers) {
  const original = await page.locator('.topology-container').elementHandle();
  const wide = page.locator('.topology-wide');
  const compact = page.locator('.compact-flow');
  const nav = page.locator('.main-nav');
  const bottom = page.locator('.mobile-nav');
  const lastDot = compact.getByRole('group', { name: '妙播音箱光点' }).locator('button').last();

  async function resize(width, height, sideNav, wideDiagram) {
    await page.setViewportSize({ width, height });
    await page.waitForFunction(([sideNav, wideDiagram]) => {
      const shown = selector => document.querySelector(selector).getClientRects().length > 0;
      return shown('.main-nav') === sideNav && shown('.topology-wide') === wideDiagram;
    }, [sideNav, wideDiagram]);
    assert.equal(await bottom.isVisible(), !sideNav);
    assert.equal(await compact.isVisible(), !wideDiagram);
    assert(await original.evaluate(node => node.isConnected), 'resizing remounted the topology');
    await fits(page);
  }

  assert(await nav.isVisible() && await wide.isVisible());
  await resize(960, 810, true, false);   // Sidebar still fits; the diagram already needs the dot view.
  await resize(761, 810, true, false);
  await resize(760, 810, false, false);
  await resize(680, 390, false, false); // A narrow landscape window uses the same layout.
  await resize(390, 910, false, false);
  await lastDot.click();
  await resize(1440, 910, true, true);
  await resize(1440, 810, true, true);
  await resize(390, 810, false, false);
  assert.equal(await compact.locator('figcaption strong').innerText(), speakers.at(-1));
  await lastDot.click();
  await resize(1440, 810, true, true);
  await original.dispose();
}

// fnOS embeds the app in a resizable iframe on its desktop.
async function checkFramedResize(page, url) {
  await page.route('**/desktop-fixture', route => route.fulfill({
    contentType: 'text/html',
    body: '<style>body{margin:0}iframe{display:block;width:100%;height:740px;border:0}</style><iframe title="Air2Mi" src="/"></iframe>',
  }));
  await page.goto(url + '/desktop-fixture');
  const outerWidth = await page.evaluate(() => innerWidth);
  const frame = page.frameLocator('iframe');
  const wide = frame.locator('.topology-wide');
  await wide.waitFor();
  const mounted = await wide.elementHandle();
  for (const [width, diagram, nav] of [
    [960, '.compact-flow', '.main-nav'],
    [540, '.compact-flow', '.mobile-nav'],
    [1440, '.topology-wide', '.main-nav'],
  ]) {
    await page.locator('iframe').evaluate((el, width) => { el.style.width = width + 'px'; }, width);
    await frame.locator(diagram).waitFor();
    await frame.locator(nav).waitFor();
    assert.equal(await page.evaluate(() => innerWidth), outerWidth);
    assert(await mounted.evaluate(node => node.isConnected));
  }
  await mounted.dispose();
}

async function checkViewport(browser, url, count, width, height) {
  const page = await browser.newPage({ viewport: { width, height }, locale: 'zh-CN', reducedMotion: 'reduce' });
  const model = snapshot(count);
  const inputs = model.inputs.map(i => i.display_name);
  const speakers = model.speakers.map(s => s.display_name);
  const errors = [];
  page.on('pageerror', e => errors.push(e.message));
  page.setDefaultTimeout(5000);
  try {
    await page.route('**/api/**', route => route.fulfill({ json:
      new URL(route.request().url()).pathname === '/api/state' ? model : {} }));
    await page.addInitScript(native => {
      sessionStorage.setItem('air2mi-dev-key', 'layout-fixture');
      if (native) window.Air2MiNative = { async requestAsync(id, path) {
        const body = await (await fetch(path)).json();
        window.__air2miReply(id, JSON.stringify({ status: 200, body }));
      } };
    }, native);
    await page.goto(url);
    await page.locator('.topology-panel').waitFor();
    if (count === 8 && width === 1440) await checkLiveResize(page, speakers);
    if (await page.locator('.topology-wide').isVisible()) {
      await checkWideGraph(page, [...inputs, ...speakers]);
    } else {
      await checkDotMap(page, inputs, speakers);
    }
    await fits(page);
    if (screenshots && count === 8 && (width === 390 || width === 1440)) {
      const file = join(screenshots, 'eight-speakers-columns-' + width + 'x' + height + '.png');
      await page.locator('.topology-container').screenshot({ path: file, animations: 'disabled' });
      if (width === 390) {
        await page.getByRole('group', { name: '妙播音箱光点' }).locator('button').last().click();
        await page.locator('.topology-container').screenshot({
          path: join(screenshots, 'eight-speakers-columns-selected.png'), animations: 'disabled' });
      }
    }
    await checkList(page, url, 'speakers', '.speaker-row', speakers);
    await checkList(page, url, 'inputs', '.receiver-row', inputs);
    await page.goto(url + '/#/river');
    const outputs = page.locator('.two-column article').filter({ has: page.getByRole('heading', { name: '投放音箱' }) });
    await outputs.waitFor();
    assert.deepEqual(await outputs.locator('.compact-row b').allTextContents(), speakers);
    await fits(page);
    if (!native && count === 8 && width === 1440) await checkFramedResize(page, url);
    assert.deepEqual(errors, []);
    console.log(count + ' devices per side, ' + width + '×' + height + ' passed');
  } finally {
    await page.close();
  }
}

if (screenshots) await mkdir(screenshots, { recursive: true });
const server = await preview({ root: fileURLToPath(new URL('../web/', import.meta.url)), configFile: false,
  preview: { host: '127.0.0.1', port: 0 }, logLevel: 'error' });
try {
  const browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH });
  try {
    const url = 'http://127.0.0.1:' + server.httpServer.address().port;
    for (const count of [8, 24]) {
      for (const [width, height] of [[320,747], [390,910], [768,1024], [1440,810], [1920,1080]]) {
        await checkViewport(browser, url, count, width, height);
      }
    }
  } finally {
    await browser.close();
  }
} finally {
  await server.close();
}
