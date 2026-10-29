/* Витрина мок-магазина ОМА: каталог, карточка товара, демо-корзина
   (carts/sync -> quote -> checkout -> pay) через /oma-api/v1. */
(function () {
  "use strict";

  const params = new URLSearchParams(location.search);
  // База API: ?api=... | localStorage | тот же origin (запуск через mockoma).
  const API = (params.get("api") || localStorage.getItem("omaApiBase") || (location.pathname.startsWith("/shop/") ? "/shop/oma-api/v1" : "/oma-api/v1")).replace(/\/$/, "");
  document.getElementById("api-base").textContent = API;
  document.getElementById("footer-api").textContent = "API: " + API;

  const projectParam=params.get("project")||"";
  const linkedProject=/^prj_[a-zA-Z0-9_-]+$/.test(projectParam)?projectParam:"";
  const DEMO_PROJECT = linkedProject || "oma-site-demo";
  const CAT_LABELS = {
    paint: "Краски и эмали",
    primer: "Грунтовки",
    coating: "Лаки, масла, пропитки",
    wallpaper: "Обои",
    flooring: "Напольные покрытия",
    tiling: "Плитка и затирка",
    door: "Двери",
    accessory: "Плинтусы, порожки, фурнитура",
    tool: "Инструмент",
    misc: "Прочее",
  };

  const state = { q: "", category: "", products: [], cart: [] };

  const $ = (id) => document.getElementById(id);
  const grid = $("grid"), empty = $("empty"), cats = $("cats");

  async function api(path, opts) {
    const res = await fetch(API + path, opts);
    if (!res.ok) {
      let msg = "HTTP " + res.status;
      try { msg = (await res.json()).message || msg; } catch (e) { /* ignore */ }
      throw new Error(msg);
    }
    return res.json();
  }

  const money = (minor, currency) =>
    (minor / 100).toFixed(2).replace(".", ",") + " " + (currency || "BYN");

  const catLabel = (c) => CAT_LABELS[c] || c;

  /* ---------- каталог ---------- */

  async function loadCategories() {
    try {
      const data = await api("/categories");
      for (const c of data.categories || []) {
        const b = document.createElement("button");
        b.className = "cats__item";
        b.dataset.category = c.category;
        b.textContent = `${catLabel(c.category)} (${c.count})`;
        b.addEventListener("click", () => setCategory(c.category));
        cats.appendChild(b);
      }
    } catch (e) {
      console.error("categories:", e);
    }
  }

  async function loadProducts() {
    grid.innerHTML = "";
    empty.hidden = true;
    const qs = new URLSearchParams();
    if (state.q) qs.set("q", state.q);
    if (state.category) qs.set("category", state.category);
    $("list-meta").textContent = "загрузка…";
    try {
      const data = await api("/products?" + qs.toString());
      state.products = data.products || [];
    } catch (e) {
      grid.innerHTML = `<div class="msg" style="grid-column:1/-1">API недоступен: ${e.message}. Запустите mockoma: <code>make oma-site</code></div>`;
      $("list-meta").textContent = "";
      return;
    }
    $("list-title").textContent = state.q
      ? `Поиск: «${state.q}»`
      : state.category ? catLabel(state.category) : "Все товары";
    $("list-meta").textContent = state.products.length + " товаров";
    for (const p of state.products) grid.appendChild(renderCard(p));
    empty.hidden = state.products.length > 0;
  }

  function renderCard(p) {
    const el = document.createElement("article");
    el.className = "card";
    const img = p.image_url
      ? `<img class="card__img" loading="lazy" src="${esc(p.image_url)}" alt="${esc(p.title)}">`
      : `<div class="card__img">нет фото</div>`;
    const old = p.old_price_minor
      ? `<span class="card__price-old">${money(p.old_price_minor, p.currency)}</span>` : "";
    const badge = p.stock > 0
      ? `<span class="card__badge">в наличии</span>`
      : `<span class="card__badge card__badge--na">нет в наличии</span>`;
    el.innerHTML = `
      <div class="card__imgbox">${img}</div>
      <div class="card__body">
        ${p.brand ? `<span class="card__brand">${esc(p.brand)}</span>` : ""}
        <p class="card__title">${esc(p.title)}</p>
        ${badge}
        <div class="card__row">
          <span class="card__price">${money(p.price_minor, p.currency)}</span>
          ${old}
        </div>
        <span class="card__cat">${esc(catLabel(p.category))}${p.oma_category ? " · " + esc(p.oma_category) : ""}</span>
      </div>`;
    el.addEventListener("click", () => openProduct(p.sku));
    return el;
  }

  /* ---------- карточка товара ---------- */

  async function openProduct(sku) {
    let p;
    try { p = await api("/products/" + encodeURIComponent(sku)); }
    catch (e) { alert("Товар недоступен: " + e.message); return; }
    const old = p.old_price_minor
      ? `<span class="pd__price-old">${money(p.old_price_minor, p.currency)}</span>` : "";
    $("product-body").innerHTML = `
      <div class="pd">
        <div class="pd__imgbox">${p.image_url ? `<img class="pd__img" src="${esc(p.image_url)}" alt="${esc(p.title)}">` : "нет фото"}</div>
        <div>
          <h2 class="pd__title">${esc(p.title)}</h2>
          <div class="pd__meta">
            ${p.brand ? `<span>Бренд: <b>${esc(p.brand)}</b></span>` : ""}
            ${p.country ? `<span>Страна: ${esc(p.country)}</span>` : ""}
            <span>Категория: ${esc(catLabel(p.category))}</span>
            <span>Ед.: ${esc(p.unit)}</span>
            <span>В упак.: ${esc(String(p.pack_amount))} ${esc(p.unit)}</span>
            <span>Остаток: ${p.stock}</span>
          </div>
          <div class="pd__price">${money(p.price_minor, p.currency)}${old}</div>
          <p class="pd__desc">${esc(p.description || "Описание отсутствует.")}</p>
          <div class="pd__actions">
            <input class="pd__qty" id="pd-qty" type="number" min="1" step="1" value="1">
            <button class="btn btn--primary" id="pd-add">${linkedProject?"Изменить в проекте →":"В корзину"}</button>
          </div>
          <div class="pd__source">
            Артикул OMA: ${esc(p.sku)} ·
            <a href="${esc(p.source_url)}" target="_blank" rel="noreferrer noopener">источник на oma.by</a>
          </div>
        </div>
      </div>`;
    $("product-modal").hidden = false;
    $("pd-add").addEventListener("click", () => {
      const packs = Math.max(1, parseInt($("pd-qty").value, 10) || 1);
      addToCart(p, packs);
    });
  }

  /* ---------- корзина: sync -> quote -> checkout -> pay ---------- */

  function addToCart(p, packs) {
if(linkedProject){location.href=`/?project=${encodeURIComponent(DEMO_PROJECT)}#cart`;return;}
    const found = state.cart.find((l) => l.sku === p.sku);
    if (found) found.packs += packs;
    else state.cart.push({ sku: p.sku, title: p.title, price: p.price_minor, currency: p.currency, unit: p.unit, image: p.image_url, packs });
    $("cart-count").textContent = String(state.cart.reduce((n, l) => n + l.packs, 0));
    $("cart-count").hidden = false;
    openCart();
  }

  function openCart() {
    const body = $("cart-body");
    if (!state.cart.length) {
      body.innerHTML = `<div class="cart-empty">Корзина пуста</div>`;
      $("cart-modal").hidden = false;

      return;
    }
    const lines = state.cart.map((l, i) => `
      <div class="cart-line">
        <img class="cart-line__img" src="${esc(l.image || "")}" alt="">
        <div class="cart-line__title">${esc(l.title)}<br><span style="color:var(--muted);font-size:12px">${esc(l.sku)}</span></div>
        <span class="cart-line__qty">${l.packs} × ${esc(l.unit)}</span>
        <span class="cart-line__sum">${money(l.price * l.packs, l.currency)}</span>
      </div>`).join("");
    const total = state.cart.reduce((n, l) => n + l.price * l.packs, 0);
    body.innerHTML = `
      <h2 style="margin-top:0">${linkedProject?"Материалы вашего проекта":"Ваша корзина"}</h2>
      ${linkedProject ? '<p>Корзина из ОМА.Подбор. Изменение состава и оформление доступны в проекте.</p>' : ""}${lines}
      <div class="cart-total"><span>Сумма:</span><b>${money(total)}</b></div>
      <div class="pd__actions">
        <button class="btn btn--primary" id="cart-sync">1. Синхронизировать корзину</button>
        <button class="btn btn--orange" id="cart-quote" disabled>2. Котировка</button>
        <button class="btn btn--orange" id="cart-pay" disabled>3. Оформить и оплатить</button>
      </div>
      <div class="cart-steps" id="cart-steps"></div>
      <p class="cart-note">Демо-оплата мок-API: реальные деньги не списываются, заказ создаётся с признаком demo.</p>`;
    $("cart-modal").hidden = false;
    if(linkedProject) {body.querySelector(".pd__actions").innerHTML=`<a class="btn btn--primary" href="/?project=${encodeURIComponent(DEMO_PROJECT)}#cart">Вернуться в ОМА.Подбор →</a>`;return;}

    const step = (text, ok) => {
      const el = document.createElement("div");
      if (ok) el.className = "ok";
      el.textContent = text;
      $("cart-steps").appendChild(el);
    };

    let cartId = null, quote = null, session = null;
    $("cart-sync").addEventListener("click", async () => {
      try {
        const res = await api("/carts/sync", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            project_id: DEMO_PROJECT,
            lines: state.cart.map((l, i) => ({ link_id: "site-" + i, sku: l.sku, packs: l.packs })),
          }),
        });
        cartId = res.cart_id;
        step(`корзина ${res.cart_id} v${res.cart_version}: ${res.upserted} строк, ${money(res.total_minor, res.currency)}`, true);
        $("cart-quote").disabled = false;
      } catch (e) { step("ошибка sync: " + e.message); }
    });
    $("cart-quote").addEventListener("click", async () => {
      try {
        quote = await api(`/carts/${encodeURIComponent(cartId)}/quotes`, { method: "POST" });
        step(`котировка ${quote.id}: ${money(quote.total_minor, quote.currency)}`, true);
        $("cart-pay").disabled = false;
      } catch (e) { step("ошибка quote: " + e.message); }
    });
    $("cart-pay").addEventListener("click", async () => {
      $("cart-pay").disabled = true;
      $("cart-pay").textContent = "Оформление…";
      try {
        session = await api("/checkout-sessions", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ quote_id: quote.id }),
        });
        step(`checkout-сессия ${session.id}: ${session.state}`, true);
        const order = await api(`/checkout-sessions/${encodeURIComponent(session.id)}/pay`, { method: "POST" });
        step(`заказ ${order.id}: статус ${order.status}, ${money(order.total_minor, order.currency)}`, true);
        $("cart-pay").textContent = "Оплачено ✓";
        state.cart = [];
        $("cart-count").hidden = true;
      } catch (e) {
        step("ошибка оплаты: " + e.message);
        $("cart-pay").disabled = false;
        $("cart-pay").textContent = "3. Оформить и оплатить";
      }
    });
  }

  /* ---------- утилиты ---------- */

  function esc(s) {
    return String(s == null ? "" : s)
      .replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;").replace(/'/g, "&#39;");
  }

  function setCategory(c) {
    state.category = c;
    for (const b of cats.querySelectorAll(".cats__item")) {
      b.classList.toggle("cats__item--active", (b.dataset.category || "") === c);
    }
    loadProducts();
  }

  if(params.get("cart")&&linkedProject) {
    api(`/carts/${encodeURIComponent(params.get("cart"))}`).then(async c=>{state.cart=await Promise.all(c.lines.map(async l=>{const p=await api(`/products/${encodeURIComponent(l.sku)}`);return {sku:l.sku,title:p.title,price:l.price_minor,currency:c.currency,packs:l.packs,unit:p.units_raw||p.unit,image:p.image_url};}));$("cart-count").hidden=!state.cart.length;$("cart-count").textContent=state.cart.reduce((s,l)=>s+l.packs,0);openCart();}).catch(e=>alert("Не удалось открыть корзину: "+e.message));
  }
  /* ---------- инициализация ---------- */

  $("search-form").addEventListener("submit", (e) => {
    e.preventDefault();
    state.q = $("search-input").value.trim();
    loadProducts();
  });
  for (const m of document.querySelectorAll(".modal")) {
    m.addEventListener("click", (e) => { if (e.target.hasAttribute("data-close")) m.hidden = true; });
  }
  $("cart-btn").addEventListener("click", openCart);

  loadCategories();
  loadProducts();
})();
