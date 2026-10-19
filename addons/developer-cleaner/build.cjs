const fs = require("node:fs");
const path = require("node:path");
const sharp = require("sharp");

const target = path.join(__dirname, "icons");
fs.mkdirSync(target, { recursive: true });
const lucide = path.join(__dirname, "node_modules/lucide-static");
for (const icon of ["settings", "trash-2", "plus", "copy", "download", "upload", "x", "check", "rotate-cw"]) {
  fs.copyFileSync(path.join(lucide, "icons", `${icon}.svg`), path.join(target, `${icon}.svg`));
}
fs.copyFileSync(path.join(lucide, "LICENSE"), path.join(target, "LICENSE.lucide"));

function icon(name, color, x, y, scale) {
  const source = fs.readFileSync(path.join(lucide, "icons", `${name}.svg`), "utf8");
  return `<g color="${color}" transform="translate(${x} ${y}) scale(${scale})">${source}</g>`;
}

function logo(compact = false) {
  const glyph = compact
    ? icon("code", "#65edca", 32, 33, 2.7)
    : icon("refresh-cw", "#38bdf8", 25, 25, 3.25) + icon("code", "#65edca", 43, 43, 1.75);
  return `<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128" viewBox="0 0 128 128">
    <defs>
      <linearGradient id="rim" x1="0" y1="1" x2="1" y2="0"><stop stop-color="#2788ff"/><stop offset=".48" stop-color="#38d9ef"/><stop offset=".8" stop-color="#65edca"/><stop offset="1" stop-color="#f5e76b"/></linearGradient>
      <filter id="glow" x="-50%" y="-50%" width="200%" height="200%"><feGaussianBlur stdDeviation="3"/></filter>
    </defs>
    <rect x="7" y="7" width="114" height="114" rx="27" fill="#0b1316"/>
    <rect x="16" y="16" width="96" height="96" rx="23" fill="none" stroke="url(#rim)" stroke-width="4" opacity=".65" filter="url(#glow)"/>
    <rect x="16" y="16" width="96" height="96" rx="23" fill="#0d1c20" stroke="url(#rim)" stroke-width="2"/>
    <g opacity=".38" filter="url(#glow)">${glyph}</g>
    ${glyph}
    <circle cx="102" cy="26" r="13" fill="#0d1c20"/>
    ${icon("sparkle", "#f5e76b", 92, 16, .83)}
  </svg>`;
}

async function build() {
  fs.writeFileSync(path.join(target, "logo.svg"), logo());
  for (const size of [16, 32, 48, 128, 512]) {
    await sharp(Buffer.from(logo(size === 16))).resize(size, size).png().toFile(path.join(target, `icon${size}.png`));
  }
  console.log("Built local logo and Lucide icons. Load this directory as an unpacked Chrome extension.");
}

build().catch(error => {
  console.error(error);
  process.exitCode = 1;
});