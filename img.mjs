// @ts-check
import { writeFileSync } from "fs";

/**
 *
 * @param {string} str
 * @returns {number}
 */
function fnv1a(str) {
  let hash = 0x811c9dc5;
  for (let i = 0; i < str.length; i++) {
    hash ^= str.charCodeAt(i);
    hash = Math.imul(hash, 0x01000193);
  }
  return hash >>> 0;
}

/**
 *
 * @param {number} seed
 * @returns {() => number}
 */
function xorShift32(seed) {
  let state = seed || 1;
  return () => {
    state ^= state << 13;
    state ^= state >>> 17;
    state ^= state << 5;
    state >>>= 0;
    return state / 0xffffffff;
  };
}

/**
 *
 * @param {string | undefined} seed
 * @returns {string}
 */
function genSVG(seed) {
  const seedStr = seed ?? Math.random().toString(36).slice(2);
  const rand = xorShift32(fnv1a(seedStr));

  const GRID = 5;
  const CELL = 40;
  const MARGIN = 28;
  const SIZE = GRID * CELL + MARGIN * 2;

  const hue = Math.floor(rand() * 360);
  const saturation = 55 + Math.floor(rand() * 15);
  const lightness = 45 + Math.floor(rand() * 15);
  const color = `hsl(${hue}, ${saturation}%, ${lightness}%)`;

  const rects = [];
  for (let col = 0; col < Math.ceil(GRID / 2); col++) {
    for (let row = 0; row < GRID; row++) {
      if (rand() >= 0.5) {
        rects.push(
          `  <rect x="${MARGIN + col * CELL}" y="${MARGIN + row * CELL}" width="${CELL}" height="${CELL}" fill="${color}"/>`,
        );
        const mirrorCol = GRID - 1 - col;
        if (mirrorCol !== col) {
          rects.push(
            `  <rect x="${MARGIN + mirrorCol * CELL}" y="${MARGIN + row * CELL}" width="${CELL}" height="${CELL}" fill="${color}"/>`,
          );
        }
      }
    }
  }

  return `<svg xmlns="http://www.w3.org/2000/svg" width="${SIZE}" height="${SIZE}" viewBox="0 0 ${SIZE} ${SIZE}">
  <rect width="${SIZE}" height="${SIZE}" fill="#f0f0f0"/>
${rects.join("\n")}
</svg>`;
}

const seed = process.argv[2] || Math.random().toString(36).slice(2);
const svg = genSVG(seed);
writeFileSync("avatar.svg", svg);
console.log(`Generated avatar.svg with seed "${seed}"`);
