"use strict";

const fs = require("node:fs");
const path = require("node:path");

function* walk(tokens) {
  for (const token of tokens) {
    yield token;
    yield* walk(token.children);
  }
}

function* listsWithDepth(tokens, depth = 0) {
  for (const token of tokens) {
    const isList =
      token.type === "listOrdered" || token.type === "listUnordered";
    const listDepth = isList ? depth + 1 : depth;
    if (isList) {
      yield { token, depth: listDepth };
    }
    yield* listsWithDepth(token.children, listDepth);
  }
}

function splitSlides(tokens) {
  const slides = [[]];
  for (const token of tokens) {
    if (token.type === "thematicBreak") {
      slides.push([]);
    } else {
      slides[slides.length - 1].push(token);
    }
  }
  return slides;
}

function displayWidth(text) {
  const wideChar =
    /[ᄀ-ᅟ⺀-〾ぁ-㏿㐀-䶿一-鿿ꀀ-꓏가-힣豈-﫿︰-﹏＀-｠￠-￦\u{1f300}-\u{1faff}]/u;
  let width = 0;
  for (const char of text) {
    width += wideChar.test(char) ? 2 : 1;
  }
  return width;
}

function renderedLines(block) {
  const textTypes = new Set([
    "data",
    "codeTextData",
    "characterEscapeValue",
    "characterReferenceValue",
    "autolinkProtocol",
    "autolinkEmail",
  ]);
  const hiddenTypes = new Set(["image", "resource"]);
  const lines = [{ lineNumber: block.startLine, text: "" }];
  const visit = (token) => {
    if (hiddenTypes.has(token.type)) {
      return;
    }
    if (
      token.type === "lineEnding" ||
      (token.type === "htmlText" && /^<br\s*\/?>$/i.test(token.text))
    ) {
      lines.push({ lineNumber: token.startLine, text: "" });
      return;
    }
    if (textTypes.has(token.type)) {
      const line = lines[lines.length - 1];
      if (line.text === "") {
        line.lineNumber = token.startLine;
      }
      line.text += token.text;
      return;
    }
    token.children.forEach(visit);
  };
  block.children.forEach(visit);
  return lines.filter((line) => line.text !== "");
}

function checkListItems(slide, limits, report) {
  const items = [...walk(slide.tokens)].filter(
    (token) => token.type === "listItemPrefix",
  );
  if (items.length > limits.maxListItems) {
    report(
      items[limits.maxListItems].startLine,
      `${items.length} list items (max ${limits.maxListItems})`,
    );
  }
}

function checkListDepth(slide, limits, report) {
  const deepest = [...listsWithDepth(slide.tokens)].reduce(
    (found, list) => (found && found.depth >= list.depth ? found : list),
    undefined,
  );
  if (deepest && deepest.depth > limits.maxListDepth) {
    report(
      deepest.token.startLine,
      `list nesting depth ${deepest.depth} (max ${limits.maxListDepth})`,
    );
  }
}

function checkLineWidth(slide, limits, report) {
  const line = { label: "line", max: limits.maxLineWidth };
  const heading = { label: "heading", max: limits.maxHeadingWidth };
  const widthLimits = new Map([
    ["paragraph", line],
    ["tableHeader", line],
    ["tableData", line],
    ["atxHeadingText", heading],
    ["setextHeadingText", heading],
  ]);
  for (const block of walk(slide.tokens)) {
    const limit = widthLimits.get(block.type);
    if (!limit) {
      continue;
    }
    for (const rendered of renderedLines(block)) {
      const width = displayWidth(rendered.text.trim());
      if (width > limit.max) {
        report(
          rendered.lineNumber,
          `${limit.label} width ${width} (max ${limit.max})`,
        );
      }
    }
  }
}

function checkBold(slide, limits, report) {
  const spans = [...walk(slide.tokens)].filter(
    (token) => token.type === "strong",
  );
  if (spans.length > limits.maxBold) {
    report(
      spans[limits.maxBold].startLine,
      `${spans.length} bold spans (max ${limits.maxBold})`,
    );
  }
}

function checkTables(slide, limits, report) {
  for (const table of walk(slide.tokens)) {
    if (table.type !== "table") {
      continue;
    }
    const rows = [...walk(table.children)].filter(
      (token) => token.type === "tableRow",
    );
    const columns = rows[0].children.filter(
      (token) => token.type === "tableHeader",
    ).length;
    const bodyRows = rows.length - 1;
    if (columns > limits.maxTableColumns) {
      report(
        table.startLine,
        `table has ${columns} columns (max ${limits.maxTableColumns})`,
      );
    }
    if (bodyRows > limits.maxTableRows) {
      report(
        table.startLine,
        `table has ${bodyRows} body rows (max ${limits.maxTableRows})`,
      );
    }
  }
}

function checkCode(slide, limits, report) {
  for (const code of walk(slide.tokens)) {
    const lines =
      code.type === "codeFenced"
        ? code.endLine - code.startLine - 1
        : code.type === "codeIndented"
          ? code.endLine - code.startLine + 1
          : 0;
    if (lines > limits.maxCodeLines) {
      report(
        code.startLine,
        `code block has ${lines} lines (max ${limits.maxCodeLines})`,
      );
    }
  }
}

function childText(token, type) {
  const child = [...walk(token.children)].find((found) => found.type === type);
  return child ? child.text : "";
}

function measureSvg(file) {
  const text = fs.readFileSync(file, "utf8");
  const svgTag = /<svg\b[^>]*>/.exec(text)?.[0] ?? "";
  const attribute = (name) => {
    const match = new RegExp(
      `\\s${name}\\s*=\\s*["']([\\d.]+)(?:px)?["']`,
    ).exec(svgTag);
    return match ? Number(match[1]) : undefined;
  };
  const viewBox =
    /\sviewBox\s*=\s*["']\s*[-\d.]+[\s,]+[-\d.]+[\s,]+([\d.]+)[\s,]+([\d.]+)/.exec(
      svgTag,
    );
  const fontSizes = [
    ...text.matchAll(/font-size\s*[:=]\s*["']?\s*([\d.]+)(?:px)?(?=["';\s}])/g),
  ].map((match) => Number(match[1]));
  const unitsWidth = viewBox ? Number(viewBox[1]) : attribute("width");
  const natural = {
    width: attribute("width") ?? unitsWidth,
    height: attribute("height") ?? (viewBox ? Number(viewBox[2]) : undefined),
  };
  if (!natural.width || !natural.height || !unitsWidth) {
    return undefined;
  }
  return {
    natural,
    textSizeAt: fontSizes.length
      ? (renderedWidth) => (Math.min(...fontSizes) * renderedWidth) / unitsWidth
      : undefined,
  };
}

function measurePng(file) {
  const header = fs.readFileSync(file).subarray(0, 24);
  if (header.toString("latin1", 1, 4) !== "PNG") {
    return undefined;
  }
  const natural = {
    width: header.readUInt32BE(16),
    height: header.readUInt32BE(20),
  };
  return {
    natural,
    scaleAt: (renderedWidth) => renderedWidth / natural.width,
  };
}

function renderedImageWidth(keywords, natural) {
  const slide = { width: 1280, height: 720, contentWidth: 1120 };
  const length = (names) => {
    for (const keyword of keywords) {
      const match = /^(\w+):([\d.]+)(?:px)?$/.exec(keyword);
      if (match && names.includes(match[1])) {
        return Number(match[2]);
      }
    }
    return undefined;
  };
  const width = length(["w", "width"]);
  const height = length(["h", "height"]);
  if (width !== undefined) {
    return width;
  }
  if (height !== undefined) {
    return (height * natural.width) / natural.height;
  }
  if (keywords.includes("bg")) {
    const split = keywords
      .map((keyword) => /^(?:left|right)(?::([\d.]+)%)?$/.exec(keyword))
      .find(Boolean);
    const boxWidth = split
      ? (slide.width * Number(split[1] ?? 50)) / 100
      : slide.width;
    const fit =
      keywords.includes("contain") || keywords.includes("fit")
        ? Math.min
        : Math.max;
    return (
      natural.width *
      fit(boxWidth / natural.width, slide.height / natural.height)
    );
  }
  return Math.min(natural.width, slide.contentWidth);
}

function checkImages(slide, limits, report) {
  if (!slide.deckDir) {
    return;
  }
  const measures = new Map([
    [".svg", measureSvg],
    [".png", measurePng],
  ]);
  for (const image of walk(slide.tokens)) {
    if (image.type !== "image") {
      continue;
    }
    const source = childText(image, "resourceDestinationString");
    const file = path.resolve(slide.deckDir, source);
    const measure = measures.get(path.extname(file).toLowerCase());
    const measured = measure && fs.existsSync(file) ? measure(file) : undefined;
    if (!measured) {
      continue;
    }
    const keywords = childText(image, "labelText").split(/\s+/);
    const width = renderedImageWidth(keywords, measured.natural);
    if (measured.textSizeAt) {
      const textSize = Number(measured.textSizeAt(width).toFixed(1));
      if (textSize < limits.minImageTextPx) {
        report(
          image.startLine,
          `image text ${textSize}px after scaling (min ${limits.minImageTextPx}): ${source}`,
        );
      }
    }
    if (measured.scaleAt) {
      const scale = measured.scaleAt(width);
      if (scale < limits.minImageScale) {
        report(
          image.startLine,
          `image scaled to ${scale.toFixed(2)} (min ${limits.minImageScale}): ${source}`,
        );
      }
    }
  }
}

module.exports = {
  names: ["slide-density"],
  description: "Slide text density",
  tags: ["marp"],
  parser: "micromark",
  function: (params, onError) => {
    const mode = params.frontMatterLines.some((line) =>
      /^class:.*\bhandout\b/.test(line),
    )
      ? "handout"
      : "projection";
    const limits = params.config[mode];
    if (!limits) {
      throw new Error(`slide-density: missing "${mode}" limits in config`);
    }
    const checks = [
      checkListItems,
      checkListDepth,
      checkLineWidth,
      checkBold,
      checkTables,
      checkCode,
      checkImages,
    ];
    const deckDir = process.env.MARP_DECK_DIR;
    splitSlides(params.parsers.micromark.tokens).forEach((tokens, index) => {
      const slide = { number: index + 1, tokens, deckDir };
      const report = (lineNumber, detail) =>
        onError({ lineNumber, detail: `slide ${slide.number}: ${detail}` });
      for (const check of checks) {
        check(slide, limits, report);
      }
    });
  },
};
