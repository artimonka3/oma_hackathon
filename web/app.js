"use strict";

let appConfig = { ai_mode: "demo", shop_connected: false, storage: "file" };
const androidDevice = /Android/i.test(navigator.userAgent);
const arIntent = "intent://#Intent;scheme=arconfig;package=com.spatial.arconfig;end";
const arLaunch = (cls="btn") => androidDevice ? `<a class="${cls}" href="${arIntent}">${arIcon()} Открыть AR просмотр ↗</a>` : `<p class="hint">AR-просмотр мебели доступен в приложении на Android. Откройте эту страницу на телефоне.</p>`;
const shopLink = () => appConfig.shop_connected ? `<a class="text-button shop-link" href="/shop/${data?.cart ? `?cart=${encodeURIComponent(data.cart.id)}&project=${encodeURIComponent(pid)}` : ""}">Открыть каталог ОМА ↗</a>` : "";
const productPhoto = (p) => /^https?:\/\//i.test(p?.image_url||"") ? `<img class="product-photo" src="${esc(p.image_url)}" alt="" loading="lazy" referrerpolicy="no-referrer" onerror="this.hidden=true">` : "";
let materialLimit=12;
let materialCategory = "all",
  materialQuery = "";
const arIcon = () =>
  `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="M3 8V3h5m8 0h5v5m0 8v5h-5M8 21H3v-5M7 9l5-3 5 3v6l-5 3-5-3zM7 9l5 3 5-3m-5 3v6"/></svg>`;
const materialTypes = {
  paint: "Краска",
  flooring: "Пол",
  tiling: "Плитка",
};
const unitLabel = (u) => ({ l: "л", m2: "м²", kg: "кг", pack: "уп." })[u] || u;
function palette(p) {
  return p.sku.includes("CREAM")
    ? "#e5d5be"
    : p.sku.includes("SAGE")
      ? "#a7b2a0"
      : p.sku.includes("TAUPE")
        ? "#c8bbb0"
        : p.material === "flooring"
          ? "#c7a879"
          : p.material === "tiling"
            ? "#d4d1c7"
            : "#f1efe8";
}
function mainMaterials() {
  return catalog.filter((p) => materialTypes[p.material] && p.stock > 0 && p.calculation_supported !== false);
}
function initLots() {
  if (!Array.isArray(draft.lots)) {
    draft.lots = (data?.inventory || [])
      .filter((x) => x.status === "confirmed")
      .map((x) => ({ sku: x.sku, amount: x.amount }));
    if (!draft.lots.length && draft.lotSKU)
      draft.lots = [{ sku: draft.lotSKU, amount: draft.amount || "" }];
  }
}
function arInvitation(compact = false) {
  return `<a class="ar-invitation ${compact ? "compact" : ""}" href="${androidDevice?arIntent:"#ar"}"><span class="ar-icon">${arIcon()}</span><span><small>ОТДЕЛЬНЫЙ ИНСТРУМЕНТ ДЛЯ ВАШЕГО ДОМА</small><b>Как мебель впишется в комнату?</b><span>Расставляйте мебель в вашем пространстве</span></span><strong>${androidDevice?"Открыть AR просмотр ↗":"Подробнее об AR →"}</strong></a>`;
}
function materialGrid() {
  const items = mainMaterials().filter(
    (p) =>
      (materialCategory === "all" || p.material === materialCategory) &&
      p.title.toLowerCase().includes(materialQuery.toLowerCase()),
  );
  return (
    items.slice(0,materialLimit)
      .map((p) => {
        const selected = draft.lots.some((l) => l.sku === p.sku);
        return `<button type="button" class="material-card ${selected ? "selected" : ""}" data-material="${p.sku}" aria-pressed="${selected}"><span class="material-sample ${p.material}" style="--sample:${palette(p)}">${productPhoto(p)}<span>${selected ? "✓" : "+"}</span></span><span class="material-copy"><small>${materialTypes[p.material]} · ${p.pack_amount} ${unitLabel(p.unit)} / ${esc(p.sales_unit||"уп.")}</small><b>${esc(p.title)}</b><span>${money(p.price_minor)} <small>/ ${esc(p.sales_unit||"уп.")}</small></span><em>${selected ? "Добавлено в мои остатки" : "У меня есть этот материал"}</em></span></button>`;
      })
       .join("") + (items.length>materialLimit ? `<button type="button" class="btn ghost materials-more" data-action="more-materials">Ещё материалы · найдено ${items.length}</button>` : "") ||
    '<div class="empty"><h3>Ничего не нашли</h3><p>Попробуйте другое название или категорию.</p></div>'
  );
}

const $ = (s) => document.querySelector(s),
  esc = (v) =>
    String(v ?? "").replace(
      /[&<>"']/g,
      (c) =>
        ({
          "&": "&amp;",
          "<": "&lt;",
          ">": "&gt;",
          '"': "&quot;",
          "'": "&#39;",
        })[c],
    ),
  money = (n) =>
    new Intl.NumberFormat("ru-BY", {
      style: "currency",
      currency: "BYN",
    }).format((n || 0) / 100);
let token = "",
  role = "client",
  pid = localStorage.getItem("oma-live-project") || "",
  data = null,
  catalog = [],
  analysis = null,
  busy = false,
  checkout = null,
  projects = [],
  draft = {};
try {
  draft = JSON.parse(sessionStorage.getItem("oma-live-brief") || "{}");
} catch {}
const root = () => `/api/v1/projects/${pid}`,
  route = () => location.hash.slice(1) || "welcome",
  go = (r) => {
    location.hash = r;
  },
  saveDraft = () =>
    sessionStorage.setItem("oma-live-brief", JSON.stringify(draft));
const label = {
  planned: "Запланирован",
  in_progress: "В работе",
  submitted: "Ожидает приёмки",
  changes_requested: "Нужны исправления",
  accepted: "Принят",
  in_review: "На проверке",
  approved: "Комплект подтверждён",
  needs_info: "Нужны уточнения",
  needs_visit: "Нужен осмотр",
  superseded: "Комплект изменился",
  declined: "Проверка отклонена",
  paid: "Оплачен",
  delivered: "Доставлен",
};
function toast(text, error = false) {
  const el = $("#notice");
  el.textContent = text;
  el.className = "visible" + (error ? " error" : "");
  clearTimeout(toast.timer);
  toast.timer = setTimeout(() => (el.className = ""), error ? 10000 : 4500);
}
async function api(path, body, method) {
  const opt = {
    method: method || (body === undefined ? "GET" : "POST"),
    headers: { Authorization: `Bearer ${token}` },
  };
  if (body instanceof FormData) opt.body = body;
  else if (body !== undefined) {
    opt.headers["Content-Type"] = "application/json";
    opt.body = JSON.stringify(body);
  }
  const res = await fetch(path, opt);
  let out;
  try {
    out = await res.json();
  } catch {
    throw Error("Сервер вернул некорректный ответ");
  }
  if (!res.ok) throw Error(out.message || `Ошибка ${res.status}`);
  return out;
}
async function session(participant) {
  const res = await fetch("/api/v1/demo/session", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ participant }),
  });
  const out = await res.json();
  if (!res.ok) throw Error(out.message || "Не удалось открыть демо-сессию");
  token = out.token;
}
async function refresh() {
  if (pid) data = await api(root() + "/application");
  if(data){const skus=[...(data.inventory||[]).map(x=>x.sku),...(data.selection?.skus||[]),...(data.specification?.new_purchase||[]).map(x=>x.sku)];for(const sku of new Set(skus)){if(!catalog.some(p=>p.sku===sku)){try{catalog.push(await api(`/api/v1/catalog/${encodeURIComponent(sku)}`));}catch{}}}}
  projects = (await api("/api/v1/projects")).projects || [];
  analysis = null;
}
async function run(task) {
  if (busy) return;
  busy = true;
  document.querySelectorAll("button").forEach((b) => {
    b.dataset.previousDisabled = String(b.disabled);
    b.disabled = true;
  });
  try {
    await task();
    await refresh();
    await render();
  } catch (e) {
    toast(e.message, true);
  } finally {
    busy = false;
    document.querySelectorAll("button").forEach((b) => {
      if (b.dataset.previousDisabled !== undefined) {
        b.disabled = b.dataset.previousDisabled === "true";
        delete b.dataset.previousDisabled;
      }
    });
  }
}
const button = (text, action, secondary = false, extra = "") =>
    `<button class="btn${secondary ? " secondary" : ""}" data-action="${action}" ${extra}>${text}</button>`,
  link = (text, to, secondary = false) =>
    `<a class="btn${secondary ? " secondary" : ""}" href="#${to}">${text}</a>`,
  pill = (text, green = false) =>
    `<span class="pill${green ? " green" : ""}">${esc(text)}</span>`;
const intro = (title, text, kicker = "ОМА.Подбор") =>
  `<div class="intro"><div class="eyebrow">${kicker}</div><h1>${title}</h1><p>${text}</p></div>`;
const steps = (i) =>
  `<div class="steps">${["Ваша задача", "Что уже есть", "Комната"].map((x, k) => `<span class="${i === k ? "current" : ""}"><i>${k + 1}</i>${x}</span>`).join("")}</div>`;
function summary() {
  const sp = data?.specification;
  return `<aside class="card"><img class="summary-image" src="/assets/living-room.jpg" alt="Пример светлого интерьера"><span class="eyebrow">Ваш комплект</span><div class="amount">${money(sp?.total_minor)}</div><p class="hint">Материалы · каталог ОМА · демо-покупка</p>${sp?.used_inventory ? `<div class="note">Учтено вашего материала: ${sp.used_inventory.toFixed(2)} ${unitLabel(catalog.find(p=>p.sku===data.selection.skus?.[0])?.unit||"м²")}</div>` : ""}<div class="line"><span>Исполнение</span><b>${data?.meta.foreman ? "С прорабом" : "Самостоятельно"}</b></div><div class="line"><span>Помещение</span><b>${esc(data?.rooms?.[0]?.name || "Не задано")}</b></div><p class="hint spacer">Работы оплачиваются отдельно по договорённости с исполнителем. Доставка в демонстрации бесплатная.</p></aside>`;
}
function projectHistory() {
  return `<details class="history"><summary>История проекта · ${(data?.events || []).length} событий</summary>${(
    data?.events || []
  )
    .slice(-12)
    .reverse()
    .map(
      (e) =>
        `<div>${new Date(e.created_at).toLocaleString("ru")} · ${esc(eventName(e.type))}</div>`,
    )
    .join("")}</details>`;
}
function eventName(t) {
  return (
    {
      "project.created": "Создан проект",
      "room.updated": "Обновлены размеры",
      "inventory.confirmed": "Подтверждены остатки",
      "specification.calculated": "Пересчитаны материалы",
      "cart.synced": "Обновлена корзина",
      "review.created": "Комплект передан прорабу",
      "review.approved": "Прораб подтвердил комплект",
      "review.needs_info": "Прораб запросил уточнения",
      "plan.proposed": "Подготовлен план работ",
      "plan.confirmed": "Подтверждён план",
      "stage.started": "Начат этап",
      "stage.report_submitted": "Получен отчёт",
      "order.paid": "Оплачен демозаказ",
      "order.delivered": "Материалы доставлены",
      "project.completed": "Проект завершён",
    }[t] || "Обновление проекта"
  );
}
function welcome() {
  if (data?.order || data?.meta.finished) {
    draft = {};
    saveDraft();
  }
  return `<section class="welcome-layout"><div class="welcome-copy"><span class="hero-label"><i></i> ОМА.ПОДБОР · ВАШ ПЛАН РЕМОНТА</span><h1>Не знаете,<br>что вам нужно?<span>Начнём с вашей идеи.</span></h1><p>Расскажите о комнате. Подберём материалы, учтём остатки и поможем найти тех, кто всё сделает.</p><div class="hero-stats"><span><b>01</b> Опишите задачу</span><span><b>02</b> Выберите материалы</span><span><b>03</b> Увидьте результат</span></div><div class="welcome-scene"><img src="/assets/living-room.jpg" alt="Светлая гостиная с натуральными материалами"><span class="scene-caption">Ваш дом. Ваши решения.</span><a href="${androidDevice?arIntent:"#ar"}" class="scene-ar">${arIcon()}<span><b>Расставьте мебель в AR</b><small>Проверьте, как мебель впишется в комнату ↗</small></span></a></div></div><form class="card form-card welcome-form" id="brief"><span class="form-step">ШАГ 1 ИЗ 3</span><h2>Что будем менять?</h2><p>Сложные названия материалов знать необязательно.</p><label class="field">Комната и задача<textarea name="goal" required maxlength="1500" placeholder="Хочу покрасить стены в гостиной. Нравятся светлые, тёплые оттенки…">${esc(draft.goal || "")}</textarea></label><label class="field">Как назовём комнату?<input name="name" required maxlength="80" placeholder="Например, гостиная" value="${esc(draft.name || "")}"></label><button class="btn wide">Собрать мой проект <span>→</span></button><p class="form-privacy">Размеры и бюджет можно будет уточнить на следующих шагах.</p>${projects.length ? link("Продолжить сохранённый проект", "projects", true) : ""}</form></section>`;
}
function leftovers() {
  initLots();
  return (
    steps(1) +
    intro(
      "Новое начинается<br>с того, что уже есть.",
      "Осталась краска или несколько упаковок ламината? Добавьте их — мы учтём пригодный остаток в расчёте.",
    ) +
    `<form id="leftovers" class="materials-layout"><section class="material-browser"><div class="material-toolbar"><label class="search-field"><span>⌕</span><input id="material-search" type="search" value="${esc(materialQuery)}" placeholder="Найти материал по названию" aria-label="Поиск материалов"></label><div class="filter-chips" role="group" aria-label="Категории материалов">${[["all", "Все материалы"], ...Object.entries(materialTypes)].map(([key, title]) => `<button type="button" class="chip ${materialCategory === key ? "active" : ""}" data-category="${key}" aria-pressed="${materialCategory === key}">${title}</button>`).join("")}</div></div><div id="material-grid" class="material-grid">${materialGrid()}</div><p class="hint">Материалы и фото из каталога вашего магазина ОМА. Цены и остатки для хакатона; перед покупкой уточните расход и единицы продажи.</p></section><aside class="material-sidebar" id="stock-panel"><div class="card"><span class="eyebrow">МОИ ОСТАТКИ</span><h2>${draft.lots.length ? (draft.lots.length === 1 ? "1 материал добавлен" : draft.lots.length + (draft.lots.length < 5 ? " материала добавлено" : " материалов добавлено")) : "Начать можно с нуля"}</h2>${
      draft.lots
        .map((l) => {
          const pr = catalog.find((p) => p.sku === l.sku);
          if (!pr) return "";
          return `<div class="lot-row"><span class="lot-swatch" style="background:${palette(pr)}"></span><div><b>${esc(pr.title)}</b><label>Доступно, ${unitLabel(pr.unit)}<input data-lot-amount="${l.sku}" name="amount-${l.sku}" type="number" min="0.01" max="100000" step="0.01" value="${esc(l.amount)}" placeholder="0" required></label></div><button type="button" class="remove-lot" data-material="${l.sku}" aria-label="Убрать ${esc(pr.title)}">×</button></div>`;
        })
        .join("") ||
      "<p>Если подходящих остатков нет, просто продолжайте. Подберём всё необходимое.</p>"
    }${draft.lots.length ? '<label class="check"><input name="confirmed" type="checkbox" required>Материалы есть в наличии и пригодны для использования</label>' : ""}<label class="field">Что ещё сохранить?<textarea name="context" rows="2" maxlength="500" placeholder="Например, диван и цвет дверей">${esc(draft.context || "")}</textarea></label><button class="btn wide">${draft.lots.length ? "Учесть остатки и продолжить" : "Продолжить без остатков"} →</button><a class="back-link" href="#welcome">← К описанию задачи</a></div>${arInvitation(true)}</aside></form><button type="button" class="mobile-stock-jump" data-action="show-stock"><span>Мои материалы · ${draft.lots.length}</span><b>Дальше →</b></button>`
  );
}
function room() {
  const rm = data?.rooms?.[0];
  return (
    steps(2) +
    intro(
      "Ваша комната.<br>Ваш точный расчёт.",
      "Введите размеры вручную для расчёта материалов. А в отдельной AR-комнате можно будет расставить мебель.",
    ) +
    `<div class="room-layout"><section class="ar-feature"><div class="ar-feature-top"><span class="pill">ФИШКА ОМА.ПОДБОР</span><span class="ar-live-dot">AR</span></div><h2>Мебель.<br>На своём месте.</h2><p>Отдельный AR-инструмент поможет расставить мебель и посмотреть на неё в вашей комнате.</p><a href="#ar" class="btn ar-launch">${arIcon()} Открыть AR-комнату ↗</a><div class="ar-room-visual" aria-hidden="true"><img src="/assets/living-room.jpg" alt=""><span class="visual-tag">Пример расстановки мебели</span></div><small>Расстановка мебели открывается в приложении на Android. Размеры для материалов вводятся вручную.</small></section><form class="card form-card measurement-card" id="room"><span class="form-step">РУЧНОЙ ВВОД</span><h2>Уже знаете размеры?</h2><p>Этого достаточно, чтобы подобрать количество материалов.</p><div class="room-fields">${[
      ["length", "Длина, м", rm?.length_mm],
      ["width", "Ширина, м", rm?.width_mm],
      ["height", "Высота, м", rm?.height_mm],
    ]
      .map(
        ([n, l, v]) =>
          `<label class="field">${l}<input name="${n}" type="number" min="0.5" max="${n === "height" ? 10 : 100}" step="0.01" value="${v ? v / 1000 : draft[n] || ""}" placeholder="${n === "height" ? "2.7" : "4.0"}" required></label>`,
      )
      .join(
        "",
      )}</div><label class="field upload">Фото комнаты, необязательно<input name="photo" type="file" accept="image/jpeg,image/png,image/webp"><span class="hint">До 10 МБ · только участникам проекта</span></label><label class="check"><input name="confirmed" type="checkbox" required>Размеры проверены, можно использовать их для расчёта</label><button class="btn wide">Подобрать материалы →</button><button type="button" class="text-button wide" data-action="preview">Сначала посмотреть идеи без размеров</button></form></div>`
  );
}
function variants() {
  const list = (data?.draft?.variants || []).filter(
    (v) => v.id !== "variant_balance",
  );
  const live = data?.draft?.mode === "live";
  return `<div class="results-heading">${intro("Идеи, которые станут домом.", "Выберите подходящую отделку. Сервер рассчитает количество единиц продажи с учётом комнаты и ваших остатков.")}<span class="ai-badge">✧ ${live ? "Подбор с AI" : "Готовые решения"}</span></div>${!data?.rooms?.length ? '<div class="note warning">Сейчас это примеры. Для точного расчёта нужны реальные размеры. ' + link("Добавить размеры", "room", true) + "</div>" : ""}<div class="variant-grid">${list
    .map((v, i) => {
      const pr = catalog.find((p) => p.sku === v.skus[0]);
      if (!pr) return "";
      return `<article class="variant-card ${data?.selection?.variant_id === v.id ? "selected" : ""}"><div class="variant-visual" style="--finish:${palette(pr)}"><img src="/assets/${i % 2 ? "kitchen" : "living-room"}.jpg" alt="Визуальный ориентир для отделки"><span class="variant-number">0${i + 1}</span><span class="finish-chip" style="background:${palette(pr)}"></span></div><div class="variant-body"><span class="eyebrow">${materialTypes[pr.material]} · ${pr.pack_amount} ${unitLabel(pr.unit)} / уп.</span><h2>${esc(v.title)}</h2><p>${esc(v.summary)}</p><div class="variant-product"><span class="lot-swatch" style="background:${palette(pr)}">${productPhoto(pr)}</span><b>${esc(pr.title)}</b></div><div class="variant-price"><strong>${money(pr.price_minor)}</strong><span>за ${esc(pr.sales_unit||"уп.")}</span></div>${button("Выбрать и рассчитать", "select", false, `data-id="${esc(v.id)}"`)}</div></article>`;
    })
    .join(
      "",
    )}</div>${!list.length ? empty("Подберём решение вместе", "Вернитесь к описанию задачи и уточните вид работ.", "welcome", "Уточнить задачу") : ""}<p class="visual-disclaimer">Фото — готовые визуальные ориентиры, не генерация вашей комнаты. ${live ? "AI помогает выбрать материал; размеры, суммы и закупку считает сервер." : "AI сейчас в деморежиме. После настройки ключа подбор будет выполнять GLM."}</p>${(data?.draft?.questions || []).length ? '<div class="note">' + data.draft.questions.map(esc).join("<br>") + "</div>" : ""}${arInvitation()}`;
}
function cart() {
  if (data?.order) return delivery();
  const sp = data?.specification;
  if (!sp)
    return empty(
      "Комплект пока не собран",
      "Выберите материалы и подтвердите размеры.",
      "variants",
      "К подбору",
    );
  return (
    intro(
      "Ваш комплект материалов",
      "Расчёт выполнен сервером. Можно увеличить количество; для уменьшения уточните размеры или остатки.",
    ) +
    `<div class="grid"><div><section class="card">${(sp.new_purchase || []).map((p) => `<div class="line"><div><b>${esc(p.title)}</b><p>${money(p.price_minor)} / ${esc(catalog.find(x=>x.sku===p.sku)?.sales_unit||"уп.")}</p></div><label class="field" style="margin:0">Количество<input aria-label="Количество: ${esc(p.title)}" class="quantity" data-quantity="${p.sku}" type="number" min="${sp.required_packs[p.sku]}" max="999" value="${p.packs}"></label><b>${money(p.total_minor)}</b></div>`).join("") || "<h2>Всё необходимое уже есть</h2><p>Подтверждённые остатки полностью покрывают потребность. Новая закупка не требуется.</p>"}<div class="actions">${link("Изменить размеры", "room", true)}${link("Остатки", "leftovers", true)}</div></section><section class="card spacer"><h2>Как будете делать ремонт?</h2><p>Покупайте материалы для самостоятельной работы или передайте комплект специалисту.</p><div class="actions">${sp.total_minor ? button("Купить самостоятельно", "self") : ""}${link("Перейти в ОМА.Прораб ↗", "prorab", true)}</div></section></div>${summary()}</div>`
  );
}
function prorab() {
  if (!data?.specification)
    return empty(
      "Начнём с вашей корзины",
      "Сначала соберите комплект материалов. На его основе подберём нужного специалиста.",
      "welcome",
      "Подобрать материалы",
    );
  return (
    intro(
      "Хороший ремонт начинается<br>с понятной задачи.",
      "Посмотрели состав вашей корзины. Вот какие работы понадобятся и кому можно доверить проверку комплекта.",
      "ОМА.Прораб",
    ) +
    `<section class="card"><div class="status-header"><h2>Что нужно сделать</h2>${pill("Анализ корзины готов", true)}</div><ul class="list">${Object.values(
      analysis?.works || {},
    )
      .map((w) => `<li>${esc(w)}</li>`)
      .join(
        "",
      )}</ul><p class="hint">Объём предварительно рассчитан по размерам комнаты. Состояние основания и сроки уточните с прорабом.</p></section><div class="specialists-heading"><h2>Подходящие специалисты</h2><p>Подобраны по материалам в вашей корзине. Все профили и портреты демонстрационные.</p></div><div class="three">${(
      analysis?.foremen || []
    )
      .map(
        ({ foreman: f, match_reasons: reasons }) =>
          `<article class="card foreman-card"><div class="foreman-photo" role="img" aria-label="Демонстрационный портрет ${esc(f.name)}" style="--portrait-x:${((f.portrait_index||0)%4)*100/3}%;--portrait-y:${Math.floor((f.portrait_index||0)/4)*80}%"></div><div class="person"><div><h3>${esc(f.name)}</h3><p class="foreman-role">${esc(f.role||"Мастер отделочных работ")}</p></div></div><p class="foreman-bio">${esc(f.bio||"")}</p><p class="hint">${esc(f.region)} · ${f.experience_years||5} лет опыта · команда ${f.team_size} чел.<br>${esc(f.availability)}</p><ul class="list">${reasons.map((x) => `<li>${esc(x)}</li>`).join("")}</ul><p class="hint">${esc(f.conditions)}</p>${button(data.meta.foreman === f.id ? "Продолжить с этим прорабом" : "Выбрать прораба", "assign", false, `data-id="${f.id}"`)}</article>`,
      )
      .join(
        "",
      )}</div>${!analysis?.foremen?.length ? '<div class="note warning">В демокаталоге нет специалиста для этого набора работ.</div>' : ""}`
  );
}
function review() {
  const rev = (data.reviews || [])
    .filter((r) => r.foreman_id === data.meta.foreman)
    .at(-1);
  return (
    intro(
      "Проверим комплект до покупки",
      "Прораб проверяет выбранные товары и объём. Решение относится к конкретному составу: после изменений потребуется новая проверка.",
      "ОМА.Прораб",
    ) +
    `<div class="grid"><section class="card">${rev ? `${pill(label[rev.status] || rev.status, rev.status === "approved")}<h2 class="spacer">${rev.status === "approved" ? "Комплект подтверждён" : "Проверка выбранным прорабом"}</h2><p>${esc(rev.comment || "Ожидаем решение специалиста.")}</p>` : "<h2>Передать комплект на проверку</h2><p>Будут переданы задача, размеры, остатки и рассчитанные материалы.</p>"}${rev?.status === "approved" ? (data.specification.total_minor && !data.order ? link("К оформлению", "checkout") : button("Перейти к плану работ", "make-plan")) : button(rev && rev.status !== "in_review" ? "Отправить актуальный комплект" : "Отправить на проверку", "send-review", false, rev?.status === "in_review" ? "disabled" : "")}<div class="actions">${button("Открыть кабинет прораба", "foreman-role", true)}${button("Обновить статус", "refresh", true)}</div>${link("Вернуться к материалам", "cart", true)}</section>${summary()}</div>`
  );
}
function checkoutScreen() {
  if (data.order) return delivery();
  return (
    intro(
      "Подтвердите покупку",
      "Оформление и сумма проверяются сервером. Оплата демонстрационная, деньги не списываются.",
      data.meta.foreman ? "ОМА.Прораб" : "ОМА.Подбор",
    ) +
    `<div class="grid"><form class="card" id="checkout"><h2>Материалы для ${esc(data.rooms?.[0]?.name || "комнаты")}</h2>${(data.specification?.new_purchase || []).map((p) => `<div class="line"><span>${esc(p.title)} × ${p.packs}</span><b>${money(p.total_minor)}</b></div>`).join("")}<label class="check"><input name="confirmed" type="checkbox" required>Подтверждаю размеры, состав и количество материалов</label><button class="btn wide">${checkout ? "Оплатить демозаказ" : "Получить итоговую котировку"}</button>${checkout ? `<div class="note">Котировка ${esc(checkout.quote.id)}<br>Итого ${money(checkout.quote.total_minor)} · действует до ${new Date(checkout.quote.expires_at).toLocaleTimeString("ru")}</div>` : ""}<p class="hint spacer">Контакты и банковские данные для демонстрации не требуются.</p></form>${summary()}</div>`
  );
}
function delivery() {
  if (!data.order)
    return empty(
      "Заказ ещё не оформлен",
      "Перейдите к подтверждению покупки.",
      "checkout",
      "Оформить заказ",
    );
  return (
    intro(
      "Материалы на пути к вам",
      "Это демонстрационный заказ магазина ОМА.",
      data.meta.foreman ? "ОМА.Прораб" : "ОМА.Подбор",
    ) +
    `<div class="grid"><section class="card">${pill(label[data.order.status] || data.order.status, true)}<h2 class="spacer">Заказ ${esc(data.order.id)}</h2><div class="amount">${money(data.order.total_minor)}</div><p>Зафиксированная сумма заказа</p><ul class="list">${(data.order.deliveries || []).map((d) => `<li>${esc(label[d.status] || "Комплектация заказа")} · ${new Date(d.at).toLocaleString("ru")}</li>`).join("")}</ul><div class="actions">${data.order.status !== "delivered" ? button("Смоделировать доставку", "deliver") : data.meta.foreman ? button("Подготовить план работ", "make-plan") : link("Мои проекты", "projects")}</div>${!data.meta.foreman ? `<p class="hint spacer">Вы выбрали самостоятельное выполнение. Можно ${'<a href="#prorab">пригласить прораба</a>'}.</p>` : ""}</section>${summary()}</div>`
  );
}
function plan() {
  const pv = data.plans?.[0];
  return (
    intro(
      "Договоримся о порядке работ",
      "План начинает действовать только после вашего подтверждения.",
      "ОМА.Прораб",
    ) +
    `<section class="card">${pv ? `${stagesMarkup(pv.stages)}<div class="actions">${pv.plan.confirmed ? link("Перейти к ремонту", "tracking") : button("Подтверждаю план работ", "confirm-plan", false, `data-id="${pv.plan.id}"`)}</div>` : `<h2>Подготовить план</h2><p>Два понятных этапа: подготовка основания и отделочные работы.</p>${button("Составить план", "make-plan")}`}</section>`
  );
}
function stagesMarkup(stages) {
  return (stages || [])
    .map(
      (s, i) =>
        `<div class="line stage"><span class="stage-index">${s.status === "accepted" ? "✓" : i + 1}</span><div class="stage-body"><h3>${esc(s.title)}</h3><p>${s.weight}% объёма · ${label[s.status] || s.status}</p></div></div>`,
    )
    .join("");
}
function tracking() {
  if (!data.plans?.length) return plan();
  return (
    intro(
      data.meta.finished ? "Ремонт завершён" : "Ваш ремонт — по шагам",
      "Прогресс растёт после приёмки выполненного объёма заказчиком.",
      "ОМА.Прораб",
    ) +
    `<div class="grid"><section class="card"><div class="status-header"><h2>Принято работ</h2><span class="amount">${data.progress}%</span></div><div class="progress"><span style="width:${data.progress}%"></span></div>${(data.stages || []).map((s, i) => `<div class="line stage"><span class="stage-index">${s.status === "accepted" ? "✓" : i + 1}</span><div class="stage-body"><h3>${esc(s.title)}</h3><p>${label[s.status] || s.status} · ${s.weight}% объёма</p>${s.status === "submitted" ? button("Посмотреть отчёт", "report-page", false, `data-id="${s.id}"`) : s.status === "accepted" ? button("История отчётов", "report-page", true, `data-id="${s.id}"`) : ""}</div></div>`).join("")}<div class="actions">${data.progress === 100 && !data.meta.finished ? button("Завершить проект", "finish") : ""}${button("Кабинет прораба", "foreman-role", true)}${button("Обновить", "refresh", true)}</div>${projectHistory()}</section>${summary()}</div>`
  );
}
let selectedStage = "";
function report() {
  const st =
    data.stages?.find((s) => s.id === selectedStage) ||
    data.stages?.find((s) => s.status === "submitted") ||
    data.stages?.at(-1);
  if (!st) return tracking();
  const reports = data.reports?.[st.id] || [];
  const rep = reports.at(-1);
  return (
    intro("Приёмка выполненных работ", esc(st.title), "ОМА.Прораб") +
    `<div class="grid"><section class="card">${rep ? `${pill("Отчёт · версия " + rep.version)}<h2 class="spacer">${esc(rep.comment)}</h2><ul class="list">${(rep.items || []).map((x) => `<li>${esc(x)}</li>`).join("")}</ul>${rep.photo_ids?.map((id) => `<img class="photo" data-photo="${id}" alt="Фотография к отчёту">`).join("") || '<p class="hint">Фото к этому отчёту не приложены.</p>'}${st.status === "submitted" ? `<form id="decision" data-stage="${st.id}" data-report="${rep.id}" data-version="${st.version}"><label class="field">Комментарий или замечание<textarea name="comment" maxlength="1500" placeholder="Что стоит исправить?"></textarea></label><label class="check"><input name="confirmed" type="checkbox">Я проверил работу и подтверждаю приёмку этого этапа</label><div class="actions"><button class="btn" name="decision" value="accept">Принять этап</button><button class="btn secondary" name="decision" value="request_changes">Отправить замечание</button></div></form>` : pill(label[st.status] || st.status, true)}<details class="history"><summary>История версий · ${reports.length}</summary>${reports.map((x) => `<p>Версия ${x.version} · ${esc(x.comment)}</p>`).join("")}</details>` : "<h2>Отчёт пока не отправлен</h2>"}<div class="actions">${link("К ремонту", "tracking", true)}</div></section>${summary()}</div>`
  );
}
function foreman() {
  const reviews = (data.reviews || []).filter(
    (r) => r.foreman_id === data.meta.foreman && r.status === "in_review",
  );
  return (
    intro(
      "Рабочий кабинет",
      "Демонстрационная роль выбранного прораба. Заказчик принимает работы в своём кабинете.",
      "ОМА.Прораб",
    ) +
    `<div class="note">Вы вошли как ${esc(analysis?.foremen?.find((f) => f.foreman.id === data.meta.foreman)?.foreman.name || data.meta.foreman)}. ${button("Вернуться к заказчику", "client-role", true)}</div><div class="grid"><div>${reviews.map((rev) => `<form class="card" id="review-decision" data-id="${rev.id}"><h2>Проверить комплект</h2><p>${esc(data.project.goal)}</p>${(data.specification?.new_purchase || []).map((p) => `<div class="line"><span>${esc(p.title)} × ${p.packs}</span><b>${money(p.total_minor)}</b></div>`).join("")}<p class="hint">${data.rooms?.map((r) => `${esc(r.name)}: ${r.length_mm / 1000} × ${r.width_mm / 1000} × ${r.height_mm / 1000} м`).join("<br>")}</p><label class="field spacer">Решение и пояснение<textarea name="comment" placeholder="Комплект подходит, объём проверен" maxlength="1500"></textarea></label><div class="actions"><button class="btn" name="decision" value="approve_current">Подтвердить комплект</button><button class="btn secondary" name="decision" value="request_info">Нужно уточнение</button></div></form>`).join("")}${!(data.stages || []).length && !reviews.length ? '<div class="card"><h2>Нет активных задач</h2><p>Заказчик может отправить комплект на проверку или подготовить план.</p></div>' : ""}${(data.stages || []).map((st, i) => `<section class="card"><span class="pill">Этап ${i + 1} · ${label[st.status] || st.status}</span><h2 class="spacer">${esc(st.title)}</h2>${st.status === "planned" ? button("Начать этап", "start-stage", false, `data-id="${st.id}"`) : ["in_progress", "changes_requested"].includes(st.status) ? `<form class="report-form" data-stage="${st.id}"><label class="field">Что сделано<textarea name="comment" maxlength="1500" required placeholder="Опишите выполненные работы и исправления"></textarea></label><label class="field">Фото результата<input name="photo" type="file" accept="image/jpeg,image/png,image/webp" required></label><button class="btn">Отправить фотоотчёт</button></form>` : st.status === "submitted" ? "<p>Ожидаем решение заказчика.</p>" : "<p>Этап принят заказчиком.</p>"}${st.status === "changes_requested" ? `<div class="note warning">${esc((data.events || []).filter((e) => e.data?.stage_id === st.id && e.data?.comment).at(-1)?.data.comment || "Проверьте замечания заказчика в истории проекта.")}</div>` : ""}</section>`).join("")}</div>${summary()}</div>`
  );
}
function projectList() {
  return (
    intro(
      "Мои проекты",
      "Ваши задачи и текущий статус ремонта.",
      role === "client" ? "ОМА.Подбор" : "ОМА.Прораб",
    ) +
    `<div class="two">${projects.map((p) => `<button class="card project" data-open="${p.id}"><span class="eyebrow">${p.execution_mode === "foreman" ? "С прорабом" : "Самостоятельно"}</span><h2>${esc(p.goal)}</h2><p class="hint">Создан ${new Date(p.created_at).toLocaleDateString("ru")} · открыть →</p></button>`).join("") || "<p>Пока нет проектов.</p>"}</div><div class="actions">${role === "client" ? button("Начать новый проект", "new", true) : ""}</div>`
  );
}
function empty(title, text, to, cta) {
  return `<div class="card empty"><h1>${title}</h1><p>${text}</p>${link(cta, to)}</div>`;
}

function arRoom() {
 return `<section class="ar-page"><div class="ar-page-copy"><span class="eyebrow">ОМА.ПОДБОР / AR-МЕБЕЛЬ</span><h1>Примерьте мебель<br><em>к своей комнате.</em></h1><p>Откройте AR-просмотр на Android и расставьте мебель в своём пространстве. Посмотрите, как предметы вписываются в ваш дом.</p><ol class="ar-steps"><li><span>01</span><div><b>Откройте приложение</b><p>Кнопка запускает установленное приложение AR Config на Android.</p></div></li><li><span>02</span><div><b>Разместите мебель</b><p>Выберите предметы и поставьте их в комнате средствами приложения.</p></div></li><li><span>03</span><div><b>Выберите расстановку</b><p>Посмотрите на мебель в своём пространстве.</p></div></li></ol>${arLaunch("btn ar-launch")}<p class="hint">Для запуска требуется установленное приложение AR Config. Размеры комнаты для расчёта материалов вводятся вручную.</p><div class="actions">${link(pid ? "Вернуться к проекту" : "Начать мой проект",pid ? (data?.specification ? "cart" : "room") : "welcome",true)}</div></div><div class="ar-stage"><div class="ar-canvas"><img src="/assets/living-room.jpg" alt="Пример гостиной с расставленной мебелью"><div class="ar-target">${arIcon()}<span>ПРЕДСТАВЬТЕ МЕБЕЛЬ В ВАШЕЙ КОМНАТЕ</span></div><span class="ar-stage-caption">Пример интерьера · просмотр открывается в приложении</span></div><div class="ar-capabilities"><span>◈ Мебель в комнате</span><span>⌑ Визуальная расстановка</span><span>✓ Ваш выбор</span></div></div></section>`;
}
async function render() {
  let r = route();
  if (!pid && !["welcome", "projects", "ar"].includes(r)) {
    go("welcome");
    return;
  }
  if (role === "foreman" && !["foreman", "projects"].includes(r)) {
    go("foreman");
    return;
  }
  const isPro =
    ["prorab", "review", "tracking", "plan", "report", "foreman"].includes(r) ||
    (data?.meta.foreman && ["checkout", "delivery"].includes(r));
  document.body.classList.toggle("intro", ["welcome"].includes(r));
  document.body.dataset.page = r;
  $("#service").textContent = isPro ? "Прораб" : "Подбор";
const storeAnchor=document.querySelector(".shop-link");if(storeAnchor)storeAnchor.href=data?.cart ? `/shop/?cart=${encodeURIComponent(data.cart.id)}&project=${encodeURIComponent(pid)}` : "/shop/";
  $(".brand").href = isPro ? "#prorab" : "#welcome";
  $("#role-label").textContent =
    role === "client" ? "Заказчик · демо" : "Прораб · демо";
  $("#switch-service").textContent = isPro
    ? "Перейти в ОМА.Подбор ↗"
    : "Перейти в ОМА.Прораб ↗";
  $("#switch-service").dataset.to = isPro ? "cart" : "prorab";
  $("#nav").innerHTML = [
    ["projects", "Мои проекты"],
    ["cart", "Корзина"],
    ...(isPro ? [["tracking", "Ремонт"]] : []),
  ]
    .map(
      ([to, t]) =>
        `<a href="#${to}" class="${r === to ? "active" : ""}">${t}</a>`,
    )
    .join("");
  if (["prorab", "foreman"].includes(r) && pid && !analysis)
    analysis = await api(root() + "/analysis");
  const screens = {
    welcome,
    ar: arRoom,
    leftovers,
    room,
    variants,
    cart,
    prorab,
    review,
    checkout: checkoutScreen,
    delivery,
    plan,
    tracking,
    report,
    foreman,
    projects: projectList,
  };
  $("#app").innerHTML = (screens[r] || welcome)();
  await loadPhotos();
}
let photoURLs = [];
async function loadPhotos() {
  photoURLs.forEach(URL.revokeObjectURL);
  photoURLs = [];
  for (const img of document.querySelectorAll("[data-photo]")) {
    try {
      const res = await fetch("/api/v1/media/" + img.dataset.photo, {
        headers: { Authorization: "Bearer " + token },
      });
      if (res.ok) {
        const url = URL.createObjectURL(await res.blob());
        photoURLs.push(url);
        img.src = url;
      }
    } catch {
      img.alt = "Не удалось загрузить фото";
    }
  }
}
async function upload(file) {
  if (!file?.size) return null;
  if (file.size > 10 * 1024 * 1024) throw Error("Выберите фото до 10 МБ");
  const form = new FormData();
  form.append("file", file);
  form.append("kind", "photo");
  return (await api(root() + "/captures", form)).id;
}
async function generate() {
  const hint = document.createElement("div");
  hint.className = "note generation-status";
  hint.setAttribute("role", "status");
  hint.textContent =
    appConfig.ai_mode === "live"
      ? "Подбираем материалы с AI. Обычно это занимает несколько секунд…"
      : "Готовим варианты для вашей комнаты…";
  document.querySelector("#app").append(hint);
  const submit = document.querySelector(
    '#room button[type="submit"], #room .btn',
  );
  const before = submit?.textContent;
  if (submit) submit.textContent = "Подбираем варианты…";
  try {
    const job = await api(root() + "/design-jobs", {});
    for (let i = 0; i < 400; i++) {
      const j = await api("/api/v1/jobs/" + job.id);
      if (j.state === "succeeded") return;
      if (j.state === "failed")
        throw Error(j.error || "Не удалось подготовить варианты");
      await new Promise((resolve) => setTimeout(resolve, 300));
    }
    throw Error(
      "Подбор ещё выполняется. Обновите страницу через несколько секунд",
    );
  } finally {
    hint.remove();
    if (submit?.isConnected) submit.textContent = before;
  }
}
async function switchRole(next) {
  if (next === "foreman" && !data?.meta.foreman)
    throw Error("Сначала выберите прораба");
  await session(next === "foreman" ? data.meta.foreman : "client");
  role = next;
  sessionStorage.setItem("oma-live-role", role);
  await refresh();
  go(next === "foreman" ? "foreman" : "review");
}
document.addEventListener("click", (event) => {
  if (event.target.closest('[data-action="show-stock"]')) {
    event.preventDefault();
    document
      .querySelector("#stock-panel")
      ?.scrollIntoView({ behavior: "smooth", block: "start" });
    return;
  }


  const material = event.target.closest("[data-material]");
  const category = event.target.closest("[data-category]");
  if (material || category) {
    event.preventDefault();
    if (busy) return;
    initLots();
    const context = document.querySelector('[name="context"]');
    if (context) draft.context = context.value;
    document.querySelectorAll("[data-lot-amount]").forEach((el) => {
      const lot = draft.lots.find((x) => x.sku === el.dataset.lotAmount);
      if (lot) lot.amount = el.value;
    });
    if (material) {
      const sku = material.dataset.material;
      const exists = draft.lots.some((x) => x.sku === sku);
      draft.lots = exists
        ? draft.lots.filter((x) => x.sku !== sku)
        : [...draft.lots, { sku, amount: "" }];
    } else materialCategory = category.dataset.category;
    saveDraft();
    render().catch((e) => toast(e.message, true));
    return;
  }

  const el = event.target.closest(
    "[data-action],[data-role],[data-open],#switch-service",
  );
  if (!el) return;
  event.preventDefault();
  run(async () => {
    if (el.dataset.role) {
      await switchRole(el.dataset.role);
      return;
    }
    if (el.dataset.open) {
      pid = el.dataset.open;
      checkout = null;
      localStorage.setItem("oma-live-project", pid);
      await refresh();
      draft = { goal: data.project.goal, name: data.rooms?.[0]?.name || "" };
      saveDraft();
      go(
        role === "foreman"
          ? "foreman"
          : data.meta.order
            ? "delivery"
            : data.specification
              ? "cart"
              : "room",
      );
      return;
    }
    if (el.id === "switch-service") {
      go(el.dataset.to);
      return;
    }
    const action = el.dataset.action;
    switch (action) {
      case "more-materials":
        materialLimit+=12;
        document.querySelector("#material-grid").innerHTML=materialGrid();
        break;
      case "new":
        materialCategory = "all";
        materialQuery = "";
        pid = "";
        data = null;
        draft = {};
        saveDraft();
        localStorage.removeItem("oma-live-project");
        go("welcome");
        break;
      case "preview":
        if (!pid) throw Error("Сначала опишите задачу");
        await generate();
        go("variants");
        break;
      case "select":
        if (!data.rooms?.length) {
          toast("Сначала подтвердите размеры комнаты");
          go("room");
          return;
        }
        await api(root() + "/design-selections", { variant_id: el.dataset.id });
        await api(root() + "/specifications", {});
        await api(root() + "/cart-sync", {});
        checkout = null;
        go("cart");
        break;
      case "self":
        await api(root() + "/foreman", { foreman_id: "" }, "PUT");
        checkout = null;
        go("checkout");
        break;
      case "assign":
        await api(root() + "/foreman", { foreman_id: el.dataset.id }, "PUT");
        checkout = null;
        go("review");
        break;
      case "send-review":
        await api(root() + "/reviews", {
          foreman_id: data.meta.foreman,
          scope: "project",
        });
        toast("Комплект отправлен прорабу");
        break;
      case "foreman-role":
        await switchRole("foreman");
        break;
      case "client-role":
        await switchRole("client");
        break;
      case "refresh":
        toast("Статус обновлён");
        break;
      case "deliver":
        await api(root() + "/demo-delivery", {});
        toast("Материалы доставлены — демонстрация");
        break;
      case "make-plan":
        await api(root() + "/default-plan", {});
        go("plan");
        break;
      case "confirm-plan":
        await api("/api/v1/work-plans/" + el.dataset.id + "/confirmation", {});
        go("tracking");
        break;
      case "start-stage":
        await api("/api/v1/stages/" + el.dataset.id + "/start", {});
        toast("Этап начат");
        break;
      case "report-page":
        selectedStage = el.dataset.id;
        go("report");
        break;
      case "finish":
        await api(root() + "/finish", {});
        toast("Проект завершён. Все этапы приняты.");
        break;
    }
  });
});
document.addEventListener("change", (event) => {
  if (event.target.dataset.quantity)
    run(async () => {
      await api(
        root() + "/cart",
        {
          sku: event.target.dataset.quantity,
          packs: Number(event.target.value),
        },
        "PATCH",
      );
      checkout = null;
      toast("Количество сохранено. Проверка комплекта потребует обновления.");
    });
});
document.addEventListener("submit", (event) => {
  event.preventDefault();
  const form = event.target,
    fd = new FormData(form),
    decision = event.submitter?.value;
  run(async () => {
    if (form.id === "brief") {
      draft.goal = fd.get("goal").trim();
      draft.name = fd.get("name").trim();
      if (!draft.goal || !draft.name) throw Error("Опишите задачу и комнату");
      saveDraft();
      if (!pid || draft.projectID !== pid || data?.order) {
        const p = await api("/api/v1/projects", {
          goal: draft.goal,
          execution_mode: "self",
        });
        pid = p.id;
        draft.projectID = pid;
        saveDraft();
        checkout = null;
        localStorage.setItem("oma-live-project", pid);
      } else {
        await api(
          root(),
          { goal: draft.goal, expected_version: data.project.version },
          "PATCH",
        );
      }
      go("leftovers");
    } else if (form.id === "leftovers") {
      initLots();
      if (draft.lots.length && !fd.get("confirmed"))
        throw Error("Подтвердите пригодность остатков");
      const items = draft.lots.map((l) => {
        const pr = catalog.find((p) => p.sku === l.sku);
        const amount = Number(fd.get("amount-" + l.sku));
        if (!pr || !Number.isFinite(amount) || amount <= 0 || amount > 100000)
          throw Error("Укажите доступное количество каждого материала");
        return {
          sku: pr.sku,
          title: pr.title,
          unit: pr.unit,
          amount: String(amount),
        };
      });
      draft.lots = items;
      draft.context = String(fd.get("context") || "");
      saveDraft();
      const goal =
        (draft.goal || data.project.goal).split("\nУчитываем:")[0] +
        (draft.context ? "\nУчитываем: " + draft.context : "");
      await api(
        root(),
        { goal, expected_version: data.project.version },
        "PATCH",
      );
      await api(root() + "/inventory", { items }, "PUT");
      go("room");
    } else if (form.id === "room") {
      await api(
        root() + "/room",
        {
          name: draft.name || data.rooms?.[0]?.name || "Комната",
          length_mm: Math.round(Number(fd.get("length")) * 1000),
          width_mm: Math.round(Number(fd.get("width")) * 1000),
          height_mm: Math.round(Number(fd.get("height")) * 1000),
          dimensions_confirmed: true,
        },
        "PUT",
      );
      await upload(fd.get("photo"));
      await generate();
      go("variants");
    } else if (form.id === "checkout") {
      if (!checkout) {
        checkout = await api(root() + "/checkout", {
          confirmed: !!fd.get("confirmed"),
        });
        if (checkout.order) {
          go("delivery");
          return;
        }
        toast("Итоговая котировка готова. Подтвердите демооплату.");
      } else {
        await api(root() + "/pay", {});
        checkout = null;
        go("delivery");
      }
    } else if (form.id === "review-decision") {
      if (decision === "request_info" && !String(fd.get("comment")).trim())
        throw Error("Напишите, что нужно уточнить");
      await api("/api/v1/reviews/" + form.dataset.id + "/decisions", {
        action: decision,
        comment: fd.get("comment") || "Комплект проверен",
      });
      toast(
        decision === "approve_current"
          ? "Комплект подтверждён"
          : "Запрошено уточнение",
      );
    } else if (form.classList.contains("report-form")) {
      const photo = await upload(fd.get("photo"));
      await api("/api/v1/stages/" + form.dataset.stage + "/reports", {
        comment: fd.get("comment"),
        items: [fd.get("comment")],
        photo_ids: photo ? [photo] : [],
      });
      toast("Фотоотчёт отправлен заказчику");
    } else if (form.id === "decision") {
      if (decision === "accept" && !fd.get("confirmed"))
        throw Error("Подтвердите, что проверили и принимаете этап");
      await api("/api/v1/stages/" + form.dataset.stage + "/decisions", {
        decision,
        comment: fd.get("comment"),
        acceptance_confirmed: !!fd.get("confirmed"),
        report_id: form.dataset.report,
        expected_version: Number(form.dataset.version),
      });
      go("tracking");
      toast(
        decision === "accept" ? "Этап принят" : "Замечание отправлено прорабу",
      );
    }
  });
});
window.addEventListener("hashchange", () => {
  if (!busy) render().catch((e) => toast(e.message, true));
  window.scrollTo(0, 0);
});
(async () => {
  try {
    appConfig = await api("/api/v1/config");
    const headerAR=document.querySelector(".header-ar");
    if(headerAR){headerAR.hidden=!androidDevice;if(androidDevice){headerAR.href=arIntent;headerAR.innerHTML=arIcon()+" Открыть AR просмотр ↗";}}
    const requestedProject=new URLSearchParams(location.search).get("project");
    if(requestedProject&&/^prj_[a-zA-Z0-9_-]+$/.test(requestedProject)){pid=requestedProject;localStorage.setItem("oma-live-project",pid);window.history.replaceState(null,"",location.pathname+location.hash);}
    document.querySelector(".demo-bar").textContent =
      `${appConfig.ai_mode === "live" ? "✧ AI-подбор подключён" : "Готовые решения · AI в деморежиме"} · Оплата демонстрационная · ${appConfig.storage === "postgres" ? "Данные в PostgreSQL" : "Локальное хранение"}`;
    document.querySelector(".header-actions").insertAdjacentHTML("afterbegin",shopLink());
    await session("client");
    catalog = (await api("/api/v1/catalog")).products || [];
    if (pid) {
      try {
        await refresh();
      } catch {
        pid = "";
        localStorage.removeItem("oma-live-project");
      }
    } else projects = (await api("/api/v1/projects")).projects || [];
    if (
      sessionStorage.getItem("oma-live-role") === "foreman" &&
      data?.meta.foreman
    ) {
      await session(data.meta.foreman);
      role = "foreman";
      await refresh();
    }
    await render();
  } catch (e) {
    $("#app").innerHTML = empty(
      "Не удалось подключиться",
      esc(e.message),
      "welcome",
      "Повторить",
    );
    toast(e.message, true);
  }
})();

document.addEventListener("input", (event) => {
  if (event.target.id === "material-search") {
    materialQuery = event.target.value;
    document.querySelector("#material-grid").innerHTML = materialGrid();
  }
  if (event.target.dataset.lotAmount) {
    const lot = draft.lots.find(
      (x) => x.sku === event.target.dataset.lotAmount,
    );
    if (lot) {
      lot.amount = event.target.value;
      saveDraft();
    }
  }
});
