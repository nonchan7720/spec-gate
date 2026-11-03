// spec-gate 承認画面。外部ライブラリは使わない。
// サーバから来た文字列は必ず textContent で入れる（innerHTML に流さない）。
"use strict";

const KIND_LABEL = {
  example: "入出力の例",
  structure: "依存の構造",
  ui: "画面",
  manual: "人が確認する項目",
};

const state = { items: [], selected: new Set(), busy: false };

function el(tag, attrs, ...children) {
  const n = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (k === "class") n.className = v;
    else if (k === "text") n.textContent = v;
    else n.setAttribute(k, v);
  }
  for (const c of children) {
    if (c == null || c === false) continue;
    n.append(c instanceof Node ? c : document.createTextNode(String(c)));
  }
  return n;
}

async function api(path, body) {
  const opt = body === undefined ? {} : {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  };
  const res = await fetch(path, opt);
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || res.status + " " + res.statusText);
  return data;
}

function showNotice(kind, text) {
  const n = document.getElementById("notice");
  n.className = "notice " + kind;
  n.textContent = text;
  n.hidden = !text;
}

async function load() {
  try {
    const data = await api("/api/pending");
    state.items = data.items || [];
    document.getElementById("who").textContent = data.by ? "承認者: " + data.by : "";
    const names = new Set(state.items.map((it) => it.name));
    for (const s of [...state.selected]) if (!names.has(s)) state.selected.delete(s);
    render();
  } catch (e) {
    showNotice("ng", "一覧を読めませんでした: " + e.message);
  }
}

function render() {
  const list = document.getElementById("list");
  list.replaceChildren();
  if (state.items.length === 0) {
    list.append(el("p", { class: "empty", text: "承認待ちの項目はありません。" }));
  }
  for (const it of state.items) list.append(card(it));
  updateButtons();
}

function updateButtons() {
  const n = state.selected.size;
  const approve = document.getElementById("approve");
  const reject = document.getElementById("reject");
  approve.disabled = state.busy || n === 0;
  reject.disabled = state.busy || n === 0;
  approve.textContent = n > 0 ? "承認（" + n + " 件）" : "承認";
  reject.textContent = n > 0 ? "却下（" + n + " 件）" : "却下";
  const all = document.getElementById("select-all");
  all.checked = state.items.length > 0 && n === state.items.length;
  all.indeterminate = n > 0 && n < state.items.length;
}

function card(it) {
  const box = el("input", { type: "checkbox", "aria-label": "選択: " + (it.title || it.name) });
  box.checked = state.selected.has(it.name);
  const c = el("section", { class: "card" + (box.checked ? " selected" : "") });
  box.addEventListener("change", () => {
    if (box.checked) state.selected.add(it.name);
    else state.selected.delete(it.name);
    c.classList.toggle("selected", box.checked);
    updateButtons();
  });

  const meta = el("div", { class: "meta" },
    it.id ? el("span", { class: "chip mono", text: it.id }) : null,
    it.kind ? el("span", { class: "chip", text: KIND_LABEL[it.kind] || it.kind }) : null,
    it.confidence ? el("span", { class: "chip" + (lowConfidence(it.confidence) ? " warn" : ""), text: "AI の一致度 " + it.confidence }) : null,
    it.area ? el("span", { class: "chip", text: "承認後の置き場所: claims/" + it.area + "/" }) : null,
  );
  const ambBtn = el("button", { type: "button", class: "btn subtle", text: "曖昧" });
  ambBtn.title = "claim の誤りではなく、仕様が決まっていないことを記録します";
  const head = el("div", { class: "card-head" }, box,
    el("div", { class: "card-title" }, el("h2", { text: it.title || it.name }), meta), ambBtn);
  c.append(head);
  const form = ambiguousForm(it);
  form.hidden = true;
  ambBtn.addEventListener("click", () => {
    form.hidden = !form.hidden;
    if (!form.hidden) form.querySelector("textarea").focus();
  });

  if (it.error) {
    c.append(el("div", { class: "err", text: "この候補は読めないため承認できません（却下・曖昧の記録のみ可能）: " + it.error }));
    c.append(form);
    return c;
  }
  c.append(form);
  c.append(el("div", { class: "cols" }, specBlock(it), el("div", {}, ...contentBlocks(it))));
  return c;
}

// ambiguousForm は「曖昧」の理由を入力させるフォーム。送ると候補は claims/_ambiguous/ に移る。
function ambiguousForm(it) {
  const id = "amb-" + it.name;
  const area = el("textarea", { id: id, rows: "2", maxlength: "500" });
  area.placeholder = "例: 月末が休日のときに前倒しするかが仕様に書かれていない";
  const send = el("button", { type: "submit", class: "btn primary", text: "曖昧として記録" });
  const cancel = el("button", { type: "button", class: "btn", text: "やめる" });
  const f = el("form", { class: "ambiguous" },
    el("label", { for: id, text: "仕様のどこが決まっていないか（必須）" }),
    area,
    el("div", { class: "ambiguous-actions" }, cancel, send));
  cancel.addEventListener("click", () => { f.hidden = true; });
  f.addEventListener("submit", async (e) => {
    e.preventDefault();
    const reason = area.value.trim();
    if (!reason) {
      showNotice("ng", "曖昧とする理由を入力してください。");
      area.focus();
      return;
    }
    await markAmbiguous(it.name, reason);
  });
  return f;
}

async function markAmbiguous(name, reason) {
  if (state.busy) return;
  state.busy = true;
  updateButtons();
  try {
    const data = await api("/api/ambiguous", { names: [name], reason: reason });
    const r = (data.results || [])[0] || {};
    if (r.ok) {
      state.selected.delete(name);
      showNotice("ok", name + " を「仕様が決まっていない」として claims/_ambiguous/ に記録しました。");
    } else {
      showNotice("ng", "✗ " + name + ": " + (r.error || "記録できませんでした"));
    }
  } catch (e) {
    showNotice("ng", "記録に失敗しました: " + e.message);
  } finally {
    state.busy = false;
    await load();
  }
}

// confidence "1/3" のように過半数に満たないものを目立たせる
function lowConfidence(s) {
  const m = /^(\d+)\/(\d+)$/.exec(s);
  return m ? Number(m[1]) < (Number(m[2]) + 1) / 2 : false;
}

function specBlock(it) {
  const b = el("div", { class: "spec" }, el("h3", { text: "仕様の該当箇所（現在の内容）" }));
  if (it.spec && it.spec.error) {
    b.append(el("div", { class: "err", text: it.spec.error }));
  } else if (it.spec && it.spec.excerpt) {
    b.append(el("pre", { text: it.spec.excerpt }));
  } else {
    b.append(el("p", { class: "small", text: "抜粋はありません。" }));
  }
  const ref = it.source && it.source.ref ? it.source.ref : "";
  b.append(el("div", { class: "ref", text: "出典: " + ref + (it.spec && it.spec.location ? "（" + it.spec.location + "）" : "") }));
  if (it.source && it.source.optional) {
    b.append(el("div", { class: "small", text: "仕様が読めなくても失敗扱いにしない設定です。" }));
  }
  return b;
}

function contentBlocks(it) {
  const out = [];
  switch (it.kind) {
    case "example":
      if (it.examples && it.examples.length) {
        out.push(el("h3", { text: "例（この入力ならこの結果になる）" }));
        const tb = el("tbody");
        for (const ex of it.examples) {
          tb.append(el("tr", {},
            el("td", { class: "mono", text: ex.in }),
            el("td", { class: "arrow", text: "→" }),
            el("td", { class: "mono", text: ex.out }),
            el("td", { text: ex.note || "" })));
        }
        out.push(el("table", {}, el("thead", {}, el("tr", {},
          el("th", { text: "入力" }), el("th", {}), el("th", { text: "期待値" }), el("th", { text: "補足" }))), tb));
      }
      if (it.properties && it.properties.length) {
        out.push(el("h3", { text: "どの入力でも成り立つこと" }));
        out.push(el("ul", { class: "plain" }, ...it.properties.map((p) => el("li", { class: "mono", text: p }))));
        out.push(el("p", { class: "small", text: "input は入力、output は結果を表します。" }));
      }
      if (it.binding) {
        out.push(el("h3", { text: "確かめ方（開発者向け）" }));
        out.push(el("div", { class: "small mono", text: it.binding }));
      }
      break;
    case "structure":
      if (it.layers && it.layers.length) {
        out.push(el("h3", { text: "層と対応するフォルダ" }));
        const tb = el("tbody");
        for (const l of it.layers) tb.append(el("tr", {}, el("td", { text: l.name }), el("td", { class: "mono", text: l.dir })));
        out.push(el("table", {}, el("thead", {}, el("tr", {}, el("th", { text: "層" }), el("th", { text: "フォルダ" }))), tb));
      }
      if (it.fromDiagram) {
        out.push(el("h3", { text: "許可する依存" }));
        out.push(el("p", { text: "図に矢印が描かれている依存だけを許可します。" }));
      }
      if (it.forbidden && it.forbidden.length) {
        out.push(el("h3", { text: "禁止する依存" }));
        out.push(el("ul", { class: "plain" }, ...it.forbidden.map((e) =>
          el("li", { text: e.raw ? e.raw : e.from + " から " + e.to + " を使ってはいけない" }))));
      }
      break;
    case "ui":
      if (it.states && it.states.length) {
        out.push(el("h3", { text: "画面の状態" }));
        out.push(el("div", { class: "meta" }, ...it.states.map((s) => el("span", { class: "chip", text: s }))));
      }
      if (it.strings && it.strings.length) {
        out.push(el("h3", { text: "画面に表示されるべき文言" }));
        out.push(el("ul", { class: "plain" }, ...it.strings.map((s) => el("li", { text: "「" + s + "」" }))));
      }
      if (it.story) {
        out.push(el("h3", { text: "対応する画面部品（開発者向け）" }));
        out.push(el("div", { class: "small mono", text: it.story }));
      }
      break;
    case "manual":
      out.push(el("h3", { text: "確認方法" }));
      out.push(el("p", { text: "機械では判定できないため、人が見て確認する項目として記録されます。" }));
      break;
  }
  if (out.length === 0) out.push(el("p", { class: "small", text: "表示できる内容がありません。" }));
  return out;
}

async function act(kind) {
  const names = state.items.map((it) => it.name).filter((n) => state.selected.has(n));
  if (names.length === 0) return;
  if (kind === "reject" && !confirm(names.length + " 件を却下します。候補ファイルは削除されます。よろしいですか？")) return;
  state.busy = true;
  updateButtons();
  try {
    const data = await api("/api/" + kind, { names });
    const results = data.results || [];
    const ok = results.filter((r) => r.ok);
    const ng = results.filter((r) => !r.ok);
    for (const r of ok) state.selected.delete(r.name);
    const verb = kind === "approve" ? "承認" : "却下";
    const lines = [];
    if (ok.length) lines.push(ok.length + " 件を" + verb + "しました。");
    for (const r of ng) lines.push("✗ " + r.name + ": " + r.error);
    showNotice(ng.length ? "ng" : "ok", lines.join("\n"));
  } catch (e) {
    showNotice("ng", "操作に失敗しました: " + e.message);
  } finally {
    state.busy = false;
    await load();
  }
}

document.addEventListener("DOMContentLoaded", () => {
  document.getElementById("approve").addEventListener("click", () => act("approve"));
  document.getElementById("reject").addEventListener("click", () => act("reject"));
  document.getElementById("select-all").addEventListener("change", (e) => {
    state.selected = new Set(e.target.checked ? state.items.map((it) => it.name) : []);
    render();
  });
  load();
});
