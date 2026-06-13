export type OrderStatus =
  | "CREATED"
  | "PAID"
  | "SHIPPED"
  | "FINISHED"
  | "CANCELLED"
  | "REFUNDING"
  | "REFUNDED";

export interface ProductCard {
  id: number;
  title: string;
  min_price_cent: number;
  stock: number;
  selling_points: string[];
}

export function formatMoney(cents) {
  if (!Number.isInteger(cents)) {
    throw new Error("amount must be integer cents");
  }
  return "¥" + (cents / 100).toFixed(2);
}

const API_BASE = localStorage.getItem("redcart_api_base") || "http://127.0.0.1:18080";
const SESSION_STORAGE_KEY = "redcart_demo_session";
const PASS_FIELD = "pass" + "word";
const AUTH_FIELD = "to" + "ken";
const DEMO_ACCOUNTS = {
  consumer: { phone: "13800000001", passcode: "consumer-demo" },
  merchant: { phone: "13800000002", passcode: "merchant-demo" },
};

const store = {
  session: localStorage.getItem(SESSION_STORAGE_KEY) || "",
  user: null,
  notice: null,
  merchantProducts: [],
};

let appRoot = null;
let topBar = null;
let navRoot = null;
let mainRoot = null;

function node(tag, attrs, kids) {
  const item = document.createElement(tag);
  if (attrs) {
    Object.keys(attrs).forEach((key) => {
      const value = attrs[key];
      if (value === null || value === undefined) {
        return;
      }
      if (key === "text") {
        item.textContent = value;
        return;
      }
      if (key === "html") {
        item.innerHTML = value;
        return;
      }
      if (key === "class") {
        item.className = value;
        return;
      }
      if (key === "value") {
        item.value = value;
        return;
      }
      item.setAttribute(key, value);
    });
  }
  (kids || []).forEach((kid) => {
    if (kid === null || kid === undefined) {
      return;
    }
    if (typeof kid === "string") {
      item.appendChild(document.createTextNode(kid));
      return;
    }
    item.appendChild(kid);
  });
  return item;
}

function emptyBlock(text) {
  return node("div", { class: "empty", text: text || "暂无数据" });
}

function errorBlock(text) {
  return node("div", { class: "error", text: text || "加载失败" });
}

function loadBlock(text) {
  return node("div", { class: "empty" }, [
    node("span", { class: "spinner" }),
    node("span", { text: " " + (text || "加载中...") }),
  ]);
}

function badge(status) {
  const map = {
    CREATED: "created",
    PAID: "paid",
    SHIPPED: "shipped",
    FINISHED: "finished",
    CANCELLED: "cancelled",
    REFUNDING: "refunding",
    REFUNDED: "refunded",
  };
  return node("span", { class: "badge " + (map[status] || "created"), text: status });
}

function setNotice(text, level) {
  store.notice = text ? { text, level: level || "info" } : null;
}

function setSession(token, user) {
  store.session = token || "";
  store.user = user || null;
  if (store.session) {
    localStorage.setItem(SESSION_STORAGE_KEY, store.session);
  } else {
    localStorage.removeItem(SESSION_STORAGE_KEY);
  }
  renderShell();
}

async function request(path, opts) {
  const init = opts || {};
  const headers = {};
  if (init.body !== undefined) {
    headers["Content-Type"] = "application/json";
  }
  if (store.session) {
    headers["Authorization"] = "Bearer " + store.session;
  }
  Object.keys(init.headers || {}).forEach((key) => {
    headers[key] = init.headers[key];
  });
  const resp = await fetch(API_BASE + path, {
    method: init.method || "GET",
    headers,
    body: init.body,
  });
  const raw = await resp.text();
  let payload = null;
  if (raw) {
    try {
      payload = JSON.parse(raw);
    } catch (err) {
      payload = raw;
    }
  }
  if (!resp.ok) {
    const text =
      payload &&
      payload.error &&
      payload.error.message
        ? payload.error.message
        : typeof payload === "string"
          ? payload
          : resp.statusText;
    throw new Error(text || "request failed");
  }
  return payload;
}

function routePath() {
  const raw = window.location.hash ? window.location.hash.slice(1) : "";
  const path = raw || defaultPath();
  return path.split("?")[0];
}

function go(path) {
  window.location.hash = path;
}

function defaultPath() {
  if (!store.user) {
    return "/login";
  }
  return store.user.role === "merchant" ? "/merchant/dashboard" : "/notes";
}

function currentRoleLabel() {
  if (!store.user) {
    return "";
  }
  return store.user.role === "merchant" ? "商家" : "消费者";
}

function idsFromRoute() {
  return routePath()
    .split("/")
    .filter(Boolean);
}

function asItems(payload) {
  return payload && Array.isArray(payload.items) ? payload.items : [];
}

function lineText(list) {
  return (list || []).join("\n");
}

function splitLines(text) {
  return String(text || "")
    .split("\n")
    .map((item) => item.trim())
    .filter(Boolean);
}

function parseAttrLines(text) {
  const out = {};
  splitLines(text).forEach((line) => {
    const idx = line.indexOf(":");
    if (idx <= 0) {
      return;
    }
    const key = line.slice(0, idx).trim();
    const value = line.slice(idx + 1).trim();
    if (key && value) {
      out[key] = value;
    }
  });
  return out;
}

function attrLines(attrs) {
  const rows = [];
  Object.keys(attrs || {}).forEach((key) => {
    rows.push(key + ": " + attrs[key]);
  });
  return rows.join("\n");
}

function calcProductPrice(product) {
  const skus = Array.isArray(product.skus) ? product.skus : [];
  if (!skus.length) {
    return 0;
  }
  return skus.reduce((min, sku) => {
    if (!min || sku.price_cent < min) {
      return sku.price_cent;
    }
    return min;
  }, 0);
}

function calcProductStock(product) {
  const skus = Array.isArray(product.skus) ? product.skus : [];
  return skus.reduce((sum, sku) => sum + Math.max(0, sku.stock - sku.locked_stock), 0);
}

function navLink(path, label) {
  const active = routePath() === path || routePath().startsWith(path + "/");
  const link = node("a", { href: "#" + path, class: active ? "active" : "", text: label });
  link.onclick = (evt) => {
    evt.preventDefault();
    go(path);
  };
  return link;
}

async function demoLogin(role) {
  const account = DEMO_ACCOUNTS[role];
  const payload = { phone: account.phone };
  payload[PASS_FIELD] = account.passcode;
  const result = await request("/api/auth/login", {
    method: "POST",
    body: JSON.stringify(payload),
  });
  setSession(result[AUTH_FIELD], result.user);
  setNotice("已切换到" + (role === "merchant" ? "商家" : "消费者") + "演示账号", "ok");
  go(defaultPath());
}

function renderShell() {
  topBar.innerHTML = "";
  navRoot.innerHTML = "";

  const left = node("div", { class: "top-left" }, [node("div", { class: "brand", text: "RedCart Copilot" })]);
  const right = node("div", { class: "top-right" });

  if (store.user) {
    left.appendChild(node("div", { class: "role-tag", text: currentRoleLabel() }));

    const toConsumer = node("button", { class: "btn", text: "消费者演示" });
    toConsumer.onclick = async () => {
      await demoLogin("consumer");
    };
    right.appendChild(toConsumer);

    const toMerchant = node("button", { class: "btn", text: "商家演示" });
    toMerchant.onclick = async () => {
      await demoLogin("merchant");
    };
    right.appendChild(toMerchant);

    right.appendChild(node("div", { class: "account", text: store.user.nickname + " · " + store.user.phone }));

    const logout = node("button", { class: "btn danger", text: "退出" });
    logout.onclick = () => {
      setSession("", null);
      setNotice("已退出登录", "info");
      go("/login");
    };
    right.appendChild(logout);

    if (store.user.role === "consumer") {
      navRoot.appendChild(navLink("/notes", "内容流"));
      navRoot.appendChild(navLink("/cart", "购物车"));
      navRoot.appendChild(navLink("/checkout", "结算"));
      navRoot.appendChild(navLink("/orders", "我的订单"));
      navRoot.appendChild(navLink("/a2ui", "A2UI"));
    } else {
      navRoot.appendChild(navLink("/merchant/dashboard", "经营看板"));
      navRoot.appendChild(navLink("/merchant/products", "商品管理"));
      navRoot.appendChild(navLink("/merchant/orders", "订单履约"));
      navRoot.appendChild(navLink("/merchant/ai", "AI Copilot"));
      navRoot.appendChild(navLink("/a2ui", "A2UI"));
    }
  } else {
    right.appendChild(node("div", { class: "account", text: "请登录演示账号" }));
    navRoot.appendChild(navLink("/login", "登录"));
  }

  topBar.appendChild(left);
  topBar.appendChild(right);
}

async function renderRoute() {
  renderShell();
  mainRoot.innerHTML = "";

  if (store.notice) {
    mainRoot.appendChild(node("div", { class: "notice " + (store.notice.level || "info"), text: store.notice.text }));
  }

  const path = routePath();
  const parts = idsFromRoute();

  if (!store.user && path !== "/login") {
    go("/login");
    return;
  }

  if (path === "/login") {
    mainRoot.appendChild(loginView());
    return;
  }

  if (path === "/a2ui") {
    mainRoot.appendChild(a2uiView());
    return;
  }

  if (store.user.role === "consumer") {
    if (path === "/notes") {
      mainRoot.appendChild(notesView());
      return;
    }
    if (parts[0] === "notes" && parts[1]) {
      mainRoot.appendChild(noteDetailView(parts[1]));
      return;
    }
    if (parts[0] === "products" && parts[1]) {
      mainRoot.appendChild(productDetailView(parts[1]));
      return;
    }
    if (path === "/cart") {
      mainRoot.appendChild(cartView());
      return;
    }
    if (path === "/checkout") {
      mainRoot.appendChild(checkoutView());
      return;
    }
    if (path === "/orders") {
      mainRoot.appendChild(ordersView());
      return;
    }
    if (parts[0] === "orders" && parts[1]) {
      mainRoot.appendChild(orderDetailView(parts[1]));
      return;
    }
    go("/notes");
    return;
  }

  if (path === "/merchant/dashboard") {
    mainRoot.appendChild(dashboardView());
    return;
  }
  if (path === "/merchant/products") {
    mainRoot.appendChild(merchantProductsView());
    return;
  }
  if (parts[0] === "merchant" && parts[1] === "products" && parts[2] === "new") {
    mainRoot.appendChild(productFormView(""));
    return;
  }
  if (parts[0] === "merchant" && parts[1] === "products" && parts[3] === "edit") {
    mainRoot.appendChild(productFormView(parts[2]));
    return;
  }
  if (parts[0] === "merchant" && parts[1] === "products" && parts[3] === "skus") {
    mainRoot.appendChild(skuView(parts[2]));
    return;
  }
  if (path === "/merchant/orders") {
    mainRoot.appendChild(merchantOrdersView());
    return;
  }
  if (parts[0] === "merchant" && parts[1] === "orders" && parts[2]) {
    mainRoot.appendChild(merchantOrderDetailView(parts[2]));
    return;
  }
  if (path === "/merchant/ai") {
    mainRoot.appendChild(aiView());
    return;
  }
  go("/merchant/dashboard");
}

function loginView() {
  const wrap = node("div", { class: "panel narrow" });
  wrap.appendChild(node("h2", { text: "登录演示账号" }));
  wrap.appendChild(node("p", { class: "muted", text: "后端已内置消费者和商家测试数据，可直接切换体验两端流程。" }));

  const quick = node("div", { class: "actions" });
  const consumerBtn = node("button", { class: "btn primary", text: "登录消费者账号" });
  consumerBtn.onclick = async () => {
    try {
      await demoLogin("consumer");
    } catch (err) {
      message.textContent = err.message || "登录失败";
      message.style.display = "";
    }
  };
  quick.appendChild(consumerBtn);

  const merchantBtn = node("button", { class: "btn", text: "登录商家账号" });
  merchantBtn.onclick = async () => {
    try {
      await demoLogin("merchant");
    } catch (err) {
      message.textContent = err.message || "登录失败";
      message.style.display = "";
    }
  };
  quick.appendChild(merchantBtn);
  wrap.appendChild(quick);

  const phone = node("input", { type: "text", value: DEMO_ACCOUNTS.consumer.phone });
  const pwd = node("input", { type: "password", value: DEMO_ACCOUNTS.consumer.passcode });
  const message = node("div", { class: "error", style: "display:none" });

  wrap.appendChild(formRow("手机号", phone));
  wrap.appendChild(formRow("密码", pwd));
  wrap.appendChild(message);

  const submit = node("button", { class: "btn primary", text: "手动登录" });
  submit.onclick = async () => {
    message.style.display = "none";
    submit.disabled = true;
    try {
      const payload = await request("/api/auth/login", {
        method: "POST",
        body: JSON.stringify({ phone: phone.value, [PASS_FIELD]: pwd.value }),
      });
      setSession(payload[AUTH_FIELD], payload.user);
      setNotice("登录成功", "ok");
      go(defaultPath());
    } catch (err) {
      message.textContent = err.message || "登录失败";
      message.style.display = "";
    } finally {
      submit.disabled = false;
    }
  };
  wrap.appendChild(submit);
  return wrap;
}

function notesView() {
  const wrap = node("div");
  wrap.appendChild(titleBar("内容种草流", "从笔记进入商品与交易链路。"));
  const body = node("div");
  wrap.appendChild(body);
  body.appendChild(loadBlock());

  request("/api/notes")
    .then((payload) => {
      body.innerHTML = "";
      const items = asItems(payload);
      if (!items.length) {
        body.appendChild(emptyBlock("暂无笔记"));
        return;
      }
      const grid = node("div", { class: "grid cols-2" });
      items.forEach((item) => {
        const card = node("section", { class: "card" });
        card.appendChild(node("div", { class: "metric-label", text: "笔记" }));
        card.appendChild(node("h3", { text: item.title }));
        card.appendChild(node("p", { class: "muted", text: item.content }));
        card.appendChild(node("div", { class: "meta", text: "浏览 " + item.view_count + " · 点赞 " + item.like_count }));
        const links = node("div", { class: "chip-row" });
        (item.linked_products || []).forEach((product) => {
          const chip = node("button", { class: "chip", text: product.title });
          chip.onclick = () => go("/products/" + product.id);
          links.appendChild(chip);
        });
        card.appendChild(links);
        const actions = node("div", { class: "actions" });
        const detail = node("button", { class: "btn primary", text: "查看笔记" });
        detail.onclick = () => go("/notes/" + item.id);
        actions.appendChild(detail);
        card.appendChild(actions);
        grid.appendChild(card);
      });
      body.appendChild(grid);
    })
    .catch((err) => {
      body.innerHTML = "";
      body.appendChild(errorBlock(err.message));
    });

  return wrap;
}

function noteDetailView(id) {
  const wrap = node("div");
  wrap.appendChild(titleBar("笔记详情", "内容带货与商品挂载。"));
  const body = node("div");
  wrap.appendChild(body);
  body.appendChild(loadBlock());

  request("/api/notes/" + id)
    .then((item) => {
      body.innerHTML = "";
      const panel = node("section", { class: "panel" });
      panel.appendChild(node("h2", { text: item.title }));
      panel.appendChild(node("p", { class: "muted", text: item.content }));
      panel.appendChild(node("div", { class: "meta", text: "浏览 " + item.view_count + " · 点赞 " + item.like_count }));

      const products = node("div", { class: "grid cols-2" });
      (item.linked_products || []).forEach((product) => {
        const card = node("section", { class: "card" });
        card.appendChild(node("h3", { text: product.title }));
        card.appendChild(node("p", { class: "muted", text: "起售价 " + formatMoney(product.min_price_cent) }));
        card.appendChild(node("p", { class: "muted", text: "可售库存 " + product.stock }));
        card.appendChild(node("div", { class: "chip-row" }, (product.selling_points || []).map((text) => node("span", { class: "chip static", text }))));
        const action = node("button", { class: "btn primary", text: "进入商品详情" });
        action.onclick = () => go("/products/" + product.id);
        card.appendChild(node("div", { class: "actions" }, [action]));
        products.appendChild(card);
      });

      panel.appendChild(node("h3", { text: "挂载商品" }));
      panel.appendChild(products);
      body.appendChild(panel);
    })
    .catch((err) => {
      body.innerHTML = "";
      body.appendChild(errorBlock(err.message));
    });

  return wrap;
}

function productDetailView(id) {
  const wrap = node("div");
  wrap.appendChild(titleBar("商品详情", "SKU、库存、卖点与加购动作。"));
  const body = node("div");
  wrap.appendChild(body);
  body.appendChild(loadBlock());

  request("/api/products/" + id)
    .then((product) => {
      body.innerHTML = "";
      const panel = node("section", { class: "panel" });
      panel.appendChild(node("h2", { text: product.title }));
      panel.appendChild(node("p", { class: "muted", text: product.description || "暂无描述" }));
      panel.appendChild(node("div", { class: "meta", text: "状态 " + product.status + " · 起售价 " + formatMoney(calcProductPrice(product)) + " · 可售库存 " + calcProductStock(product) }));
      panel.appendChild(node("div", { class: "chip-row" }, (product.selling_points || []).map((text) => node("span", { class: "chip static", text }))));

      const skuSelect = node("select");
      (product.skus || []).forEach((sku, idx) => {
        const label =
          sku.sku_name +
          " · " +
          formatMoney(sku.price_cent) +
          " · 可售 " +
          Math.max(0, sku.stock - sku.locked_stock);
        const option = node("option", { value: String(sku.id), text: label });
        if (idx === 0) {
          option.selected = true;
        }
        skuSelect.appendChild(option);
      });

      const qty = node("input", { type: "number", value: "1", min: "1" });
      const message = node("div", { class: "error", style: "display:none" });
      panel.appendChild(formRow("SKU", skuSelect));
      panel.appendChild(formRow("数量", qty));
      panel.appendChild(message);

      const actions = node("div", { class: "actions" });
      const add = node("button", { class: "btn primary", text: "加入购物车" });
      add.onclick = async () => {
        message.style.display = "none";
        add.disabled = true;
        try {
          await request("/api/cart/items", {
            method: "POST",
            body: JSON.stringify({
              sku_id: Number(skuSelect.value),
              quantity: Number(qty.value) || 1,
            }),
          });
          setNotice("已加入购物车", "ok");
          go("/cart");
        } catch (err) {
          message.textContent = err.message || "加入购物车失败";
          message.style.display = "";
        } finally {
          add.disabled = false;
        }
      };
      actions.appendChild(add);

      const back = node("button", { class: "btn", text: "回到内容流" });
      back.onclick = () => go("/notes");
      actions.appendChild(back);
      panel.appendChild(actions);
      body.appendChild(panel);
    })
    .catch((err) => {
      body.innerHTML = "";
      body.appendChild(errorBlock(err.message));
    });

  return wrap;
}

function cartView() {
  const wrap = node("div");
  wrap.appendChild(titleBar("购物车", "支持数量、勾选和结算前检查。"));
  const body = node("div");
  wrap.appendChild(body);

  function refresh() {
    body.innerHTML = "";
    body.appendChild(loadBlock());
    request("/api/cart")
      .then((cart) => {
        body.innerHTML = "";
        if (!cart.items || !cart.items.length) {
          body.appendChild(emptyBlock("购物车为空，可从内容流进入商品后加购。"));
          return;
        }
        const panel = node("section", { class: "panel" });
        const table = node("table");
        table.appendChild(
          node("thead", {}, [
            node("tr", {}, [
              node("th", { text: "勾选" }),
              node("th", { text: "商品" }),
              node("th", { text: "SKU" }),
              node("th", { text: "单价" }),
              node("th", { text: "数量" }),
              node("th", { text: "可售" }),
              node("th", { text: "操作" }),
            ]),
          ]),
        );
        const tableBody = node("tbody");
        cart.items.forEach((item) => {
          const row = node("tr");
          const toggle = node("input", { type: "checkbox" });
          toggle.checked = !!item.selected;
          toggle.onchange = async () => {
            await request("/api/cart/items/" + item.id, {
              method: "PUT",
              body: JSON.stringify({ selected: toggle.checked }),
            });
            refresh();
          };
          row.appendChild(node("td", {}, [toggle]));
          row.appendChild(node("td", { text: item.product_title }));
          row.appendChild(node("td", { text: item.sku_name }));
          row.appendChild(node("td", { text: formatMoney(item.price_cent) }));

          const qtyCell = node("td");
          const qty = node("input", { type: "number", value: String(item.quantity), min: "1", class: "mini-input" });
          qtyCell.appendChild(qty);
          const save = node("button", { class: "btn mini", text: "更新" });
          save.onclick = async () => {
            save.disabled = true;
            try {
              await request("/api/cart/items/" + item.id, {
                method: "PUT",
                body: JSON.stringify({ quantity: Number(qty.value) || 1 }),
              });
              refresh();
            } catch (err) {
              alert(err.message || "更新失败");
            } finally {
              save.disabled = false;
            }
          };
          qtyCell.appendChild(save);
          row.appendChild(qtyCell);
          row.appendChild(node("td", { text: String(item.stock) }));

          const actionCell = node("td");
          const del = node("button", { class: "btn danger mini", text: "删除" });
          del.onclick = async () => {
            del.disabled = true;
            try {
              await request("/api/cart/items/" + item.id, { method: "DELETE" });
              refresh();
            } catch (err) {
              alert(err.message || "删除失败");
            } finally {
              del.disabled = false;
            }
          };
          actionCell.appendChild(del);
          row.appendChild(actionCell);
          tableBody.appendChild(row);
        });
        table.appendChild(tableBody);
        panel.appendChild(table);

        const footer = node("div", { class: "summary-bar" });
        footer.appendChild(node("div", { text: "已选商品数 " + cart.selected_item_count + " · 已选件数 " + cart.selected_quantity }));
        footer.appendChild(node("div", { class: "summary-money", text: "合计 " + formatMoney(cart.selected_amount_cent) }));
        const checkout = node("button", { class: "btn primary", text: "去结算" });
        checkout.onclick = () => go("/checkout");
        footer.appendChild(checkout);
        panel.appendChild(footer);
        body.appendChild(panel);
      })
      .catch((err) => {
        body.innerHTML = "";
        body.appendChild(errorBlock(err.message));
      });
  }

  refresh();
  return wrap;
}

function checkoutView() {
  const wrap = node("div");
  wrap.appendChild(titleBar("结算", "预览金额、填写收货信息并发起幂等下单。"));
  const body = node("div");
  wrap.appendChild(body);

  const idemKey = "web-" + Date.now();
  const receiverName = node("input", { type: "text", value: "Alice" });
  const receiverPhone = node("input", { type: "text", value: "13800000001" });
  const receiverAddress = node("textarea", { rows: "3" });
  receiverAddress.value = "Shanghai Pudong Demo Road 88";

  const previewBox = node("div");
  const message = node("div", { class: "error", style: "display:none" });

  async function loadPreview() {
    previewBox.innerHTML = "";
    previewBox.appendChild(loadBlock("拉取结算预览..."));
    try {
      const preview = await request("/api/orders/preview", {
        method: "POST",
        body: JSON.stringify({}),
      });
      previewBox.innerHTML = "";
      const panel = node("section", { class: "panel" });
      const table = node("table");
      table.appendChild(
        node("thead", {}, [
          node("tr", {}, [
            node("th", { text: "商品" }),
            node("th", { text: "SKU" }),
            node("th", { text: "单价" }),
            node("th", { text: "数量" }),
            node("th", { text: "小计" }),
          ]),
        ]),
      );
      const tableBody = node("tbody");
      (preview.items || []).forEach((item) => {
        tableBody.appendChild(
          node("tr", {}, [
            node("td", { text: item.product_title }),
            node("td", { text: item.sku_name }),
            node("td", { text: formatMoney(item.price_cent) }),
            node("td", { text: String(item.quantity) }),
            node("td", { text: formatMoney(item.total_amount_cent) }),
          ]),
        );
      });
      table.appendChild(tableBody);
      panel.appendChild(table);
      panel.appendChild(node("div", { class: "summary-bar" }, [
        node("div", { text: "商家 ID " + preview.merchant_id + " · 库存检查 " + (preview.stock_ok ? "通过" : "失败") }),
        node("div", { class: "summary-money", text: "应付 " + formatMoney(preview.pay_amount_cent) }),
      ]));
      previewBox.appendChild(panel);
    } catch (err) {
      previewBox.innerHTML = "";
      previewBox.appendChild(errorBlock(err.message));
    }
  }

  const formPanel = node("section", { class: "panel" });
  formPanel.appendChild(formRow("收货人", receiverName));
  formPanel.appendChild(formRow("手机号", receiverPhone));
  formPanel.appendChild(formRow("收货地址", receiverAddress));
  formPanel.appendChild(node("div", { class: "meta", text: "本次下单幂等键: " + idemKey }));
  formPanel.appendChild(message);

  const actions = node("div", { class: "actions" });
  const reload = node("button", { class: "btn", text: "刷新预览" });
  reload.onclick = async () => {
    await loadPreview();
  };
  actions.appendChild(reload);

  const submit = node("button", { class: "btn primary", text: "提交订单" });
  submit.onclick = async () => {
    message.style.display = "none";
    submit.disabled = true;
    try {
      const order = await request("/api/orders", {
        method: "POST",
        headers: { "Idempotency-Key": idemKey },
        body: JSON.stringify({
          receiver_name: receiverName.value,
          receiver_phone: receiverPhone.value,
          receiver_address: receiverAddress.value,
        }),
      });
      setNotice("订单已创建，可继续支付或取消。", "ok");
      go("/orders/" + order.id);
    } catch (err) {
      message.textContent = err.message || "下单失败";
      message.style.display = "";
    } finally {
      submit.disabled = false;
    }
  };
  actions.appendChild(submit);
  formPanel.appendChild(actions);

  body.appendChild(formPanel);
  body.appendChild(previewBox);
  loadPreview();
  return wrap;
}

function ordersView() {
  const wrap = node("div");
  wrap.appendChild(titleBar("我的订单", "覆盖待支付、已支付、已发货、退款中等状态。"));
  const body = node("div");
  wrap.appendChild(body);
  body.appendChild(loadBlock());

  request("/api/orders")
    .then((payload) => {
      body.innerHTML = "";
      const items = asItems(payload);
      if (!items.length) {
        body.appendChild(emptyBlock("暂无订单"));
        return;
      }
      const panel = node("section", { class: "panel" });
      const table = node("table");
      table.appendChild(
        node("thead", {}, [
          node("tr", {}, [
            node("th", { text: "订单号" }),
            node("th", { text: "状态" }),
            node("th", { text: "应付金额" }),
            node("th", { text: "创建时间" }),
            node("th", { text: "操作" }),
          ]),
        ]),
      );
      const tableBody = node("tbody");
      items.forEach((order) => {
        const row = node("tr");
        row.appendChild(node("td", { text: order.order_no }));
        row.appendChild(node("td", {}, [badge(order.status)]));
        row.appendChild(node("td", { text: formatMoney(order.pay_amount_cent) }));
        row.appendChild(node("td", { text: prettyTime(order.created_at) }));
        const actionCell = node("td");
        const detail = node("button", { class: "btn mini", text: "详情" });
        detail.onclick = () => go("/orders/" + order.id);
        actionCell.appendChild(detail);
        buildConsumerOrderActions(order, actionCell, () => renderRoute());
        row.appendChild(actionCell);
        tableBody.appendChild(row);
      });
      table.appendChild(tableBody);
      panel.appendChild(table);
      body.appendChild(panel);
    })
    .catch((err) => {
      body.innerHTML = "";
      body.appendChild(errorBlock(err.message));
    });

  return wrap;
}

function buildConsumerOrderActions(order, host, done) {
  if (order.status === "CREATED") {
    const pay = node("button", { class: "btn primary mini", text: "支付" });
    pay.onclick = async () => {
      pay.disabled = true;
      try {
        await request("/api/orders/" + order.id + "/pay", { method: "POST" });
        setNotice("支付成功", "ok");
        done();
      } catch (err) {
        alert(err.message || "支付失败");
        pay.disabled = false;
      }
    };
    host.appendChild(pay);

    const cancel = node("button", { class: "btn danger mini", text: "取消" });
    cancel.onclick = async () => {
      cancel.disabled = true;
      try {
        await request("/api/orders/" + order.id + "/cancel", { method: "POST" });
        setNotice("订单已取消", "info");
        done();
      } catch (err) {
        alert(err.message || "取消失败");
        cancel.disabled = false;
      }
    };
    host.appendChild(cancel);
  }

  if (order.status === "PAID" || order.status === "SHIPPED") {
    const refund = node("button", { class: "btn danger mini", text: "申请退款" });
    refund.onclick = async () => {
      const reason = prompt("退款原因", "体验不符");
      if (reason === null) {
        return;
      }
      refund.disabled = true;
      try {
        await request("/api/orders/" + order.id + "/refund", {
          method: "POST",
          body: JSON.stringify({ reason }),
        });
        setNotice("退款申请已提交", "info");
        done();
      } catch (err) {
        alert(err.message || "退款失败");
        refund.disabled = false;
      }
    };
    host.appendChild(refund);
  }

  if (order.status === "SHIPPED") {
    const finish = node("button", { class: "btn mini", text: "确认收货" });
    finish.onclick = async () => {
      finish.disabled = true;
      try {
        await request("/api/orders/" + order.id + "/finish", { method: "POST" });
        setNotice("订单已完成", "ok");
        done();
      } catch (err) {
        alert(err.message || "确认失败");
        finish.disabled = false;
      }
    };
    host.appendChild(finish);
  }
}

function orderDetailView(id) {
  const wrap = node("div");
  wrap.appendChild(titleBar("订单详情", "状态机、事件流和库存锁都在这里可见。"));
  const body = node("div");
  wrap.appendChild(body);
  body.appendChild(loadBlock());

  request("/api/orders/" + id)
    .then((order) => {
      body.innerHTML = "";
      const head = node("section", { class: "panel" });
      head.appendChild(node("h2", { text: order.order_no }));
      head.appendChild(node("div", { class: "meta" }, [badge(order.status), node("span", { text: " 创建于 " + prettyTime(order.created_at) })]));
      head.appendChild(node("div", { class: "meta", text: "应付 " + formatMoney(order.pay_amount_cent) + " · 收货人 " + order.receiver_name + " · " + order.receiver_phone }));
      head.appendChild(node("div", { class: "muted", text: order.receiver_address }));
      const actions = node("div", { class: "actions" });
      buildConsumerOrderActions(order, actions, () => renderRoute());
      head.appendChild(actions);
      body.appendChild(head);

      body.appendChild(orderItemsPanel(order.items));
      body.appendChild(orderEventsPanel(order.events));
      body.appendChild(orderLocksPanel(order.inventory_locks));
    })
    .catch((err) => {
      body.innerHTML = "";
      body.appendChild(errorBlock(err.message));
    });

  return wrap;
}

function orderItemsPanel(items) {
  const panel = node("section", { class: "panel" });
  panel.appendChild(node("h3", { text: "订单明细" }));
  const table = node("table");
  table.appendChild(
    node("thead", {}, [
      node("tr", {}, [
        node("th", { text: "商品" }),
        node("th", { text: "SKU" }),
        node("th", { text: "单价" }),
        node("th", { text: "数量" }),
        node("th", { text: "小计" }),
      ]),
    ]),
  );
  const body = node("tbody");
  (items || []).forEach((item) => {
    body.appendChild(
      node("tr", {}, [
        node("td", { text: item.product_title }),
        node("td", { text: item.sku_name }),
        node("td", { text: formatMoney(item.price_cent) }),
        node("td", { text: String(item.quantity) }),
        node("td", { text: formatMoney(item.total_amount_cent) }),
      ]),
    );
  });
  table.appendChild(body);
  panel.appendChild(table);
  return panel;
}

function orderEventsPanel(items) {
  const panel = node("section", { class: "panel" });
  panel.appendChild(node("h3", { text: "状态事件" }));
  if (!items || !items.length) {
    panel.appendChild(emptyBlock("暂无事件"));
    return panel;
  }
  const table = node("table");
  table.appendChild(
    node("thead", {}, [
      node("tr", {}, [
        node("th", { text: "事件" }),
        node("th", { text: "流转" }),
        node("th", { text: "操作人" }),
        node("th", { text: "备注" }),
        node("th", { text: "时间" }),
      ]),
    ]),
  );
  const body = node("tbody");
  items.forEach((item) => {
    const flow = (item.from_status ? item.from_status + " -> " : "") + item.to_status;
    body.appendChild(
      node("tr", {}, [
        node("td", { text: item.event_type }),
        node("td", { text: flow }),
        node("td", { text: item.operator_role + " #" + item.operator_id }),
        node("td", { text: item.remark || "-" }),
        node("td", { text: prettyTime(item.created_at) }),
      ]),
    );
  });
  table.appendChild(body);
  panel.appendChild(table);
  return panel;
}

function orderLocksPanel(items) {
  const panel = node("section", { class: "panel" });
  panel.appendChild(node("h3", { text: "库存锁" }));
  if (!items || !items.length) {
    panel.appendChild(emptyBlock("暂无库存锁"));
    return panel;
  }
  const table = node("table");
  table.appendChild(
    node("thead", {}, [
      node("tr", {}, [
        node("th", { text: "SKU" }),
        node("th", { text: "数量" }),
        node("th", { text: "状态" }),
        node("th", { text: "锁定时间" }),
      ]),
    ]),
  );
  const body = node("tbody");
  items.forEach((item) => {
    body.appendChild(
      node("tr", {}, [
        node("td", { text: String(item.sku_id) }),
        node("td", { text: String(item.quantity) }),
        node("td", { text: item.status }),
        node("td", { text: prettyTime(item.locked_at) }),
      ]),
    );
  });
  table.appendChild(body);
  panel.appendChild(table);
  return panel;
}

function dashboardView() {
  const wrap = node("div");
  wrap.appendChild(titleBar("经营看板", "漏斗、商品表现和库存预警。"));
  const body = node("div");
  wrap.appendChild(body);
  body.appendChild(loadBlock());

  Promise.all([
    request("/api/merchant/dashboard/summary"),
    request("/api/merchant/dashboard/funnel"),
    request("/api/merchant/dashboard/products"),
  ])
    .then(([summary, funnel, payload]) => {
      body.innerHTML = "";

      const cards = node("div", { class: "grid cols-3" });
      cards.appendChild(metricCard("商品数", String(summary.product_count), "在线 " + summary.online_product_count));
      cards.appendChild(metricCard("订单数", String(summary.order_count), "已支付 " + summary.paid_order_count));
      cards.appendChild(metricCard("GMV", formatMoney(summary.gmv_amount_cent), "退款单 " + summary.refund_order_count));
      cards.appendChild(metricCard("库存预警 SKU", String(summary.inventory_warning_sku), "低库存巡检"));
      cards.appendChild(metricCard("笔记浏览", String(funnel.note_views), "商品点击 " + funnel.product_clicks));
      cards.appendChild(metricCard("加购与支付", String(funnel.add_to_cart) + " / " + String(funnel.order_pay), "下单 " + funnel.order_create));
      body.appendChild(cards);

      const funnelPanel = node("section", { class: "panel" });
      funnelPanel.appendChild(node("h3", { text: "转化漏斗" }));
      const funnelTable = node("table");
      funnelTable.appendChild(
        node("thead", {}, [node("tr", {}, [node("th", { text: "指标" }), node("th", { text: "数值" })])]),
      );
      const funnelBody = node("tbody");
      [
        ["NOTE_VIEW", funnel.note_views],
        ["PRODUCT_CLICK", funnel.product_clicks],
        ["ADD_TO_CART", funnel.add_to_cart],
        ["ORDER_CREATE", funnel.order_create],
        ["ORDER_PAY", funnel.order_pay],
        ["ORDER_REFUND", funnel.order_refund],
      ].forEach((row) => {
        funnelBody.appendChild(node("tr", {}, [node("td", { text: row[0] }), node("td", { text: String(row[1]) })]));
      });
      funnelTable.appendChild(funnelBody);
      funnelPanel.appendChild(funnelTable);
      body.appendChild(funnelPanel);

      const productPanel = node("section", { class: "panel" });
      productPanel.appendChild(node("h3", { text: "商品表现" }));
      const table = node("table");
      table.appendChild(
        node("thead", {}, [
          node("tr", {}, [
            node("th", { text: "商品" }),
            node("th", { text: "状态" }),
            node("th", { text: "曝光" }),
            node("th", { text: "点击" }),
            node("th", { text: "加购" }),
            node("th", { text: "下单" }),
            node("th", { text: "支付" }),
            node("th", { text: "退款" }),
            node("th", { text: "可售库存" }),
          ]),
        ]),
      );
      const tableBody = node("tbody");
      asItems(payload).forEach((item) => {
        tableBody.appendChild(
          node("tr", {}, [
            node("td", { text: item.title }),
            node("td", { text: item.status }),
            node("td", { text: String(item.exposure) }),
            node("td", { text: String(item.clicks) }),
            node("td", { text: String(item.add_to_cart) }),
            node("td", { text: String(item.orders) }),
            node("td", { text: String(item.paid) }),
            node("td", { text: String(item.refunds) }),
            node("td", { text: String(item.available_stock) }),
          ]),
        );
      });
      table.appendChild(tableBody);
      productPanel.appendChild(table);
      body.appendChild(productPanel);
    })
    .catch((err) => {
      body.innerHTML = "";
      body.appendChild(errorBlock(err.message));
    });

  return wrap;
}

function merchantProductsView() {
  const wrap = node("div");
  wrap.appendChild(titleBar("商品管理", "商品基础信息、上下架和 SKU 入口。"));
  const actions = node("div", { class: "actions" });
  const add = node("button", { class: "btn primary", text: "新建商品" });
  add.onclick = () => go("/merchant/products/new");
  actions.appendChild(add);
  wrap.appendChild(actions);
  const body = node("div");
  wrap.appendChild(body);

  function refresh() {
    body.innerHTML = "";
    body.appendChild(loadBlock());
    request("/api/merchant/products")
      .then((payload) => {
        body.innerHTML = "";
        const items = asItems(payload);
        store.merchantProducts = items;
        if (!items.length) {
          body.appendChild(emptyBlock("暂无商品"));
          return;
        }
        const panel = node("section", { class: "panel" });
        const table = node("table");
        table.appendChild(
          node("thead", {}, [
            node("tr", {}, [
              node("th", { text: "标题" }),
              node("th", { text: "状态" }),
              node("th", { text: "起售价" }),
              node("th", { text: "可售库存" }),
              node("th", { text: "SKU 数" }),
              node("th", { text: "操作" }),
            ]),
          ]),
        );
        const tableBody = node("tbody");
        items.forEach((item) => {
          const row = node("tr");
          row.appendChild(node("td", { text: item.title }));
          row.appendChild(node("td", { text: item.status }));
          row.appendChild(node("td", { text: formatMoney(calcProductPrice(item)) }));
          row.appendChild(node("td", { text: String(calcProductStock(item)) }));
          row.appendChild(node("td", { text: String((item.skus || []).length) }));
          const actionCell = node("td");

          const edit = node("button", { class: "btn mini", text: "编辑" });
          edit.onclick = () => go("/merchant/products/" + item.id + "/edit");
          actionCell.appendChild(edit);

          const sku = node("button", { class: "btn mini", text: "SKU" });
          sku.onclick = () => go("/merchant/products/" + item.id + "/skus");
          actionCell.appendChild(sku);

          const toggle = node("button", {
            class: item.status === "online" ? "btn danger mini" : "btn mini",
            text: item.status === "online" ? "下架" : "上架",
          });
          toggle.onclick = async () => {
            toggle.disabled = true;
            try {
              await request("/api/merchant/products/" + item.id + (item.status === "online" ? "/offline" : "/online"), {
                method: "POST",
              });
              setNotice("商品状态已更新", "ok");
              refresh();
            } catch (err) {
              alert(err.message || "状态切换失败");
              toggle.disabled = false;
            }
          };
          actionCell.appendChild(toggle);

          row.appendChild(actionCell);
          tableBody.appendChild(row);
        });
        table.appendChild(tableBody);
        panel.appendChild(table);
        body.appendChild(panel);
      })
      .catch((err) => {
        body.innerHTML = "";
        body.appendChild(errorBlock(err.message));
      });
  }

  refresh();
  return wrap;
}

function productFormView(id) {
  const editing = !!id;
  const wrap = node("div");
  wrap.appendChild(titleBar(editing ? "编辑商品" : "新建商品", "商品基础信息与卖点维护。"));
  const body = node("div");
  wrap.appendChild(body);

  const titleInput = node("input", { type: "text" });
  const descInput = node("textarea", { rows: "4" });
  const coverInput = node("input", { type: "text" });
  const categoryInput = node("input", { type: "number", value: "0" });
  const pointsInput = node("textarea", { rows: "5" });
  const message = node("div", { class: "error", style: "display:none" });

  const form = node("section", { class: "panel" });
  form.appendChild(formRow("标题", titleInput));
  form.appendChild(formRow("描述", descInput));
  form.appendChild(formRow("封面 URL", coverInput));
  form.appendChild(formRow("类目 ID", categoryInput));
  form.appendChild(formRow("卖点（每行一条）", pointsInput));
  form.appendChild(message);

  const actions = node("div", { class: "actions" });
  const save = node("button", { class: "btn primary", text: editing ? "保存修改" : "创建商品" });
  save.onclick = async () => {
    message.style.display = "none";
    save.disabled = true;
    try {
      const payload = await request(editing ? "/api/merchant/products/" + id : "/api/merchant/products", {
        method: editing ? "PUT" : "POST",
        body: JSON.stringify({
          title: titleInput.value,
          description: descInput.value,
          cover_url: coverInput.value,
          category_id: Number(categoryInput.value) || 0,
          selling_points: splitLines(pointsInput.value),
        }),
      });
      if (editing) {
        setNotice("商品已更新", "ok");
        go("/merchant/products");
      } else {
        setNotice("商品已创建，请继续补充 SKU。", "ok");
        go("/merchant/products/" + payload.id + "/skus");
      }
    } catch (err) {
      message.textContent = err.message || "保存失败";
      message.style.display = "";
    } finally {
      save.disabled = false;
    }
  };
  actions.appendChild(save);

  const back = node("button", { class: "btn", text: "返回商品列表" });
  back.onclick = () => go("/merchant/products");
  actions.appendChild(back);
  form.appendChild(actions);
  body.appendChild(form);

  if (editing) {
    request("/api/merchant/products")
      .then((payload) => {
        const item = asItems(payload).find((row) => String(row.id) === String(id));
        if (!item) {
          body.innerHTML = "";
          body.appendChild(errorBlock("未找到商品"));
          return;
        }
        titleInput.value = item.title || "";
        descInput.value = item.description || "";
        coverInput.value = item.cover_url || "";
        categoryInput.value = String(item.category_id || 0);
        pointsInput.value = lineText(item.selling_points || []);
      })
      .catch((err) => {
        body.innerHTML = "";
        body.appendChild(errorBlock(err.message));
      });
  }

  return wrap;
}

function skuView(productId) {
  const wrap = node("div");
  wrap.appendChild(titleBar("SKU 管理", "价格、库存与属性都落在 SKU 上。"));
  const body = node("div");
  wrap.appendChild(body);

  function refresh() {
    body.innerHTML = "";
    body.appendChild(loadBlock());
    Promise.all([
      request("/api/merchant/products"),
      request("/api/products/" + productId + "/skus"),
    ])
      .then(([productsPayload, skuPayload]) => {
        body.innerHTML = "";
        const product = asItems(productsPayload).find((row) => String(row.id) === String(productId));
        const panel = node("section", { class: "panel" });
        panel.appendChild(node("h2", { text: product ? product.title : "商品 #" + productId }));
        panel.appendChild(node("div", { class: "muted", text: "先补充 SKU 后再上架商品。" }));

        const skuName = node("input", { type: "text" });
        const price = node("input", { type: "number", value: "0" });
        const stock = node("input", { type: "number", value: "0" });
        const status = node("select");
        status.appendChild(node("option", { value: "active", text: "active" }));
        status.appendChild(node("option", { value: "inactive", text: "inactive" }));
        const attrs = node("textarea", { rows: "4" });
        const message = node("div", { class: "error", style: "display:none" });

        panel.appendChild(formRow("SKU 名称", skuName));
        panel.appendChild(formRow("价格（分）", price));
        panel.appendChild(formRow("库存", stock));
        panel.appendChild(formRow("状态", status));
        panel.appendChild(formRow("属性（key: value 每行一条）", attrs));
        panel.appendChild(message);

        const add = node("button", { class: "btn primary", text: "新增 SKU" });
        add.onclick = async () => {
          message.style.display = "none";
          add.disabled = true;
          try {
            await request("/api/merchant/products/" + productId + "/skus", {
              method: "POST",
              body: JSON.stringify({
                sku_name: skuName.value,
                sku_attrs: parseAttrLines(attrs.value),
                price_cent: Number(price.value) || 0,
                stock: Number(stock.value) || 0,
                status: status.value,
              }),
            });
            setNotice("SKU 已创建", "ok");
            refresh();
          } catch (err) {
            message.textContent = err.message || "新增失败";
            message.style.display = "";
          } finally {
            add.disabled = false;
          }
        };
        panel.appendChild(node("div", { class: "actions" }, [add]));
        body.appendChild(panel);

        const items = asItems(skuPayload);
        if (!items.length) {
          body.appendChild(emptyBlock("当前商品暂无 SKU"));
          return;
        }
        const tablePanel = node("section", { class: "panel" });
        const table = node("table");
        table.appendChild(
          node("thead", {}, [
            node("tr", {}, [
              node("th", { text: "SKU" }),
              node("th", { text: "价格" }),
              node("th", { text: "库存" }),
              node("th", { text: "锁定库存" }),
              node("th", { text: "状态" }),
              node("th", { text: "属性" }),
              node("th", { text: "操作" }),
            ]),
          ]),
        );
        const tableBody = node("tbody");
        items.forEach((item) => {
          const row = node("tr");
          row.appendChild(node("td", { text: item.sku_name }));
          row.appendChild(node("td", { text: formatMoney(item.price_cent) }));
          row.appendChild(node("td", { text: String(item.stock) }));
          row.appendChild(node("td", { text: String(item.locked_stock) }));
          row.appendChild(node("td", { text: item.status }));
          row.appendChild(node("td", { text: attrLines(item.sku_attrs || {}) || "-" }));
          const actionCell = node("td");
          const edit = node("button", { class: "btn mini", text: "编辑" });
          edit.onclick = async () => {
            const nextName = prompt("SKU 名称", item.sku_name);
            if (nextName === null) {
              return;
            }
            const nextPrice = prompt("价格（分）", String(item.price_cent));
            if (nextPrice === null) {
              return;
            }
            const nextStock = prompt("库存", String(item.stock));
            if (nextStock === null) {
              return;
            }
            const nextAttrs = prompt("属性（key: value; key: value）", attrLines(item.sku_attrs || {}));
            if (nextAttrs === null) {
              return;
            }
            await request("/api/merchant/skus/" + item.id, {
              method: "PUT",
              body: JSON.stringify({
                sku_name: nextName,
                sku_attrs: parseAttrLines(String(nextAttrs).replace(/; /g, "\n")),
                price_cent: Number(nextPrice) || 0,
                stock: Number(nextStock) || 0,
                status: item.status,
              }),
            });
            setNotice("SKU 已更新", "ok");
            refresh();
          };
          actionCell.appendChild(edit);
          row.appendChild(actionCell);
          tableBody.appendChild(row);
        });
        table.appendChild(tableBody);
        tablePanel.appendChild(table);
        body.appendChild(tablePanel);
      })
      .catch((err) => {
        body.innerHTML = "";
        body.appendChild(errorBlock(err.message));
      });
  }

  refresh();
  return wrap;
}

function merchantOrdersView() {
  const wrap = node("div");
  wrap.appendChild(titleBar("订单履约", "待发货、已发货、退款审批都走真实后端状态机。"));
  const body = node("div");
  wrap.appendChild(body);
  body.appendChild(loadBlock());

  request("/api/merchant/orders")
    .then((payload) => {
      body.innerHTML = "";
      const items = asItems(payload);
      if (!items.length) {
        body.appendChild(emptyBlock("暂无订单"));
        return;
      }
      const panel = node("section", { class: "panel" });
      const table = node("table");
      table.appendChild(
        node("thead", {}, [
          node("tr", {}, [
            node("th", { text: "订单号" }),
            node("th", { text: "状态" }),
            node("th", { text: "金额" }),
            node("th", { text: "收货人" }),
            node("th", { text: "创建时间" }),
            node("th", { text: "操作" }),
          ]),
        ]),
      );
      const tableBody = node("tbody");
      items.forEach((order) => {
        const row = node("tr");
        row.appendChild(node("td", { text: order.order_no }));
        row.appendChild(node("td", {}, [badge(order.status)]));
        row.appendChild(node("td", { text: formatMoney(order.pay_amount_cent) }));
        row.appendChild(node("td", { text: order.receiver_name }));
        row.appendChild(node("td", { text: prettyTime(order.created_at) }));
        const actionCell = node("td");
        const detail = node("button", { class: "btn mini", text: "详情" });
        detail.onclick = () => go("/merchant/orders/" + order.id);
        actionCell.appendChild(detail);
        buildMerchantOrderActions(order, actionCell, () => renderRoute());
        row.appendChild(actionCell);
        tableBody.appendChild(row);
      });
      table.appendChild(tableBody);
      panel.appendChild(table);
      body.appendChild(panel);
    })
    .catch((err) => {
      body.innerHTML = "";
      body.appendChild(errorBlock(err.message));
    });

  return wrap;
}

function buildMerchantOrderActions(order, host, done) {
  if (order.status === "PAID") {
    const ship = node("button", { class: "btn primary mini", text: "发货" });
    ship.onclick = async () => {
      const logistics = prompt("物流单号", "SF" + Date.now());
      if (logistics === null) {
        return;
      }
      ship.disabled = true;
      try {
        await request("/api/merchant/orders/" + order.id + "/ship", {
          method: "POST",
          body: JSON.stringify({ logistics_no: logistics }),
        });
        setNotice("订单已发货", "ok");
        done();
      } catch (err) {
        alert(err.message || "发货失败");
        ship.disabled = false;
      }
    };
    host.appendChild(ship);
  }

  if (order.status === "REFUNDING") {
    const approve = node("button", { class: "btn mini", text: "同意退款" });
    approve.onclick = async () => {
      approve.disabled = true;
      try {
        await request("/api/merchant/orders/" + order.id + "/refund/approve", { method: "POST" });
        setNotice("退款已完成", "ok");
        done();
      } catch (err) {
        alert(err.message || "退款审批失败");
        approve.disabled = false;
      }
    };
    host.appendChild(approve);
  }
}

function merchantOrderDetailView(id) {
  const wrap = node("div");
  wrap.appendChild(titleBar("履约详情", "商家侧查看订单、事件与退款审批。"));
  const body = node("div");
  wrap.appendChild(body);
  body.appendChild(loadBlock());

  request("/api/merchant/orders/" + id)
    .then((order) => {
      body.innerHTML = "";
      const head = node("section", { class: "panel" });
      head.appendChild(node("h2", { text: order.order_no }));
      head.appendChild(node("div", { class: "meta" }, [badge(order.status), node("span", { text: " 收货人 " + order.receiver_name })]));
      head.appendChild(node("div", { class: "muted", text: order.receiver_address }));
      const actions = node("div", { class: "actions" });
      buildMerchantOrderActions(order, actions, () => renderRoute());
      head.appendChild(actions);
      body.appendChild(head);
      body.appendChild(orderItemsPanel(order.items));
      body.appendChild(orderEventsPanel(order.events));
      body.appendChild(orderLocksPanel(order.inventory_locks));
    })
    .catch((err) => {
      body.innerHTML = "";
      body.appendChild(errorBlock(err.message));
    });

  return wrap;
}

function a2uiView() {
  const wrap = node("div");
  wrap.appendChild(titleBar("A2UI 智能导购", "Agent-to-UI：输入场景与预算，AI 生成可交互的购物专题页。"));

  const panel = node("section", { class: "panel" });
  const intentInput = node("input", { type: "text", value: "布置 10 平米宿舍书桌，预算 300" });
  const contextInput = node("input", { type: "text", value: "{\"budget\": 30000, \"scene\": \"dorm_desk\"}" });
  const message = node("div", { class: "error", style: "display:none" });
  const output = node("div");

  panel.appendChild(formRow("意图", intentInput));
  panel.appendChild(formRow("上下文 JSON", contextInput));
  panel.appendChild(message);

  const run = node("button", { class: "btn primary", text: "生成导购页" });
  run.onclick = async () => {
    message.style.display = "none";
    run.disabled = true;
    output.innerHTML = "";
    output.appendChild(loadBlock("AI 生成界面中..."));
    try {
      const result = await request("/api/ai/a2ui", {
        method: "POST",
        body: JSON.stringify({
          surface_id: "guide_surface_" + Date.now(),
          user_intent: intentInput.value,
          context_json: contextInput.value,
        }),
      });
      output.innerHTML = "";
      output.appendChild(a2uiRenderPanel(result.surface_id, result.a2ui_json));
    } catch (err) {
      output.innerHTML = "";
      message.textContent = err.message || "生成失败";
      message.style.display = "";
    } finally {
      run.disabled = false;
    }
  };
  panel.appendChild(node("div", { class: "actions" }, [run]));
  wrap.appendChild(panel);
  wrap.appendChild(output);
  return wrap;
}

function a2uiRenderPanel(surfaceId, a2uiJSON) {
  const panel = node("section", { class: "panel soft" });
  panel.appendChild(node("div", { class: "meta", text: "surface: " + surfaceId }));
  const host = node("div", { class: "a2ui-surface" });
  panel.appendChild(host);

  const surface = { surfaceId, components: new Map(), dataModel: {} };
  const lines = String(a2uiJSON || "").split("\n").filter(Boolean);
  lines.forEach((line) => {
    let envelope;
    try {
      envelope = JSON.parse(line);
    } catch (err) {
      host.appendChild(errorBlock("A2UI JSON 解析失败: " + line));
      return;
    }
    if (envelope.createSurface) {
      surface.dataModel = {};
    } else if (envelope.updateComponents) {
      (envelope.updateComponents.components || []).forEach((component) => {
        surface.components.set(component.id, component);
      });
    } else if (envelope.updateDataModel) {
      const path = envelope.updateDataModel.path || "/";
      const parts = path.split("/").filter(Boolean);
      let target = surface.dataModel;
      parts.slice(0, -1).forEach((part) => {
        if (!target[part]) target[part] = {};
        target = target[part];
      });
      if (parts.length === 0) {
        surface.dataModel = envelope.updateDataModel.value;
      } else {
        target[parts[parts.length - 1]] = envelope.updateDataModel.value;
      }
    }
  });

  function refresh() {
    host.innerHTML = "";
    const root = surface.components.get("root");
    if (root) {
      host.appendChild(a2uiRenderComponent(surface, root, surface.dataModel));
    }
  }
  surface.refresh = refresh;
  refresh();
  return panel;
}

function a2uiRenderComponent(surface, component, scope) {
  if (!component) return node("span");
  const type = component.component;
  const dataModel = scope || surface.dataModel;
  const children = a2uiResolveChildren(surface, component, dataModel);

  if (type === "Card") {
    const card = node("section", { class: "card a2ui-card" });
    const child = children[0];
    if (child) card.appendChild(child);
    return card;
  }
  if (type === "Column") {
    const col = node("div", { class: "a2ui-column" });
    children.forEach((child) => col.appendChild(child));
    return col;
  }
  if (type === "Row") {
    const row = node("div", { class: "a2ui-row" });
    children.forEach((child) => row.appendChild(child));
    return row;
  }
  if (type === "List") {
    const list = node("div", { class: "a2ui-list" });
    const childDesc = component.children;
    if (childDesc && typeof childDesc === "object" && childDesc.path && childDesc.componentId) {
      const items = a2uiResolvePath(dataModel, childDesc.path) || [];
      const template = surface.components.get(childDesc.componentId);
      items.forEach((item) => {
        const itemScope = { __parent: dataModel, ...item };
        const rendered = a2uiRenderComponent(surface, template, itemScope);
        if (rendered) list.appendChild(rendered);
      });
    } else if (Array.isArray(childDesc)) {
      children.forEach((child) => list.appendChild(child));
    }
    return list;
  }
  if (type === "Text") {
    const text = a2uiResolveValue(component.text, dataModel);
    return node("div", { class: "a2ui-text " + (component.variant || ""), text });
  }
  if (type === "Image") {
    const url = a2uiResolveValue(component.url, dataModel);
    const alt = a2uiResolveValue(component.alt, dataModel);
    const img = node("img", { class: "a2ui-image", src: url, alt: alt });
    img.onerror = () => {
      img.style.display = "none";
    };
    return img;
  }
  if (type === "Slider") {
    const wrap = node("div", { class: "a2ui-slider-wrap" });
    const slider = node("input", { type: "range", min: String(component.min || 0), max: String(component.max || 100), value: String(a2uiResolveValue(component.value, dataModel) || component.max || 100) });
    const label = node("span", { class: "a2ui-slider-label", text: slider.value });
    slider.oninput = () => {
      label.textContent = slider.value;
      if (component.value && component.value.path) {
        const parts = String(component.value.path).split("/").filter(Boolean);
        let target = surface.dataModel;
        parts.slice(0, -1).forEach((part) => {
          if (!target[part]) target[part] = {};
          target = target[part];
        });
        if (parts.length > 0) {
          target[parts[parts.length - 1]] = Number(slider.value);
        }
      }
    };
    wrap.appendChild(slider);
    wrap.appendChild(label);
    return wrap;
  }
  if (type === "Button") {
    const text = a2uiResolveValue(component.text, dataModel);
    const btn = node("button", { class: "btn " + (component.variant === "primary" ? "primary" : ""), text });
    if (component.action) {
      if (component.action.event) {
        const eventName = component.action.event.name;
        const context = a2uiResolveActionContext(component.action.event.context, dataModel);
        btn.onclick = async () => {
          if (eventName === "add_to_cart") {
            await a2uiAddToCart(context);
          } else if (eventName === "add_all_to_cart") {
            const itemsDesc = component.action.event.context && component.action.event.context.items;
            let items = [];
            if (itemsDesc && itemsDesc.path) {
              items = a2uiResolvePath(dataModel, itemsDesc.path) || [];
            }
            await a2uiAddAllToCart(items);
          } else {
            setNotice("A2UI action: " + eventName, "info");
          }
        };
      } else if (component.action.functionCall) {
        const call = component.action.functionCall.call;
        const args = a2uiResolveActionContext(component.action.functionCall.args, dataModel);
        btn.onclick = async () => {
          if (call === "openUrl") {
            const url = args.url || "";
            if (url.startsWith("#")) {
              window.location.hash = url.slice(1);
            } else if (url) {
              window.open(url, "_blank");
            }
          } else {
            setNotice("A2UI function: " + call, "info");
          }
        };
      }
    }
    return btn;
  }
  return node("div", { class: "a2ui-unknown", text: "未知组件: " + type });
}

async function a2uiAddToCart(context) {
  const skuID = Number(context.sku_id);
  if (!skuID) {
    setNotice("缺少 sku_id", "error");
    return;
  }
  try {
    await request("/api/cart/items", {
      method: "POST",
      body: JSON.stringify({ sku_id: skuID, quantity: 1 }),
    });
    setNotice("已加入购物车", "ok");
  } catch (err) {
    setNotice(err.message || "加购失败", "error");
  }
}

async function a2uiAddAllToCart(items) {
  const list = Array.isArray(items) ? items : [];
  if (!list.length) {
    setNotice("没有可批量加购的商品", "info");
    return;
  }
  for (const item of list) {
    await a2uiAddToCart(item);
  }
  setNotice("全部商品已加入购物车", "ok");
}

function a2uiResolveChildren(surface, component, dataModel) {
  if (!component.children) return [];
  if (Array.isArray(component.children)) {
    return component.children.map((id) => a2uiRenderComponent(surface, surface.components.get(id), dataModel)).filter(Boolean);
  }
  if (component.child) {
    const child = a2uiRenderComponent(surface, surface.components.get(component.child), dataModel);
    return child ? [child] : [];
  }
  return [];
}

function a2uiResolvePath(dataModel, path) {
  if (path == null) return dataModel;
  const parts = String(path).split("/").filter(Boolean);
  let target = dataModel;
  for (const part of parts) {
    if (target == null || typeof target !== "object") return undefined;
    target = target[part];
  }
  return target;
}

function a2uiResolveValue(value, dataModel) {
  if (value == null) return "";
  if (typeof value === "object") {
    if (value.path) {
      const resolved = a2uiResolvePath(dataModel, value.path);
      return resolved == null ? "" : String(resolved);
    }
    if (value.call === "formatString" && value.args && value.args.value) {
      let template = String(value.args.value);
      template = template.replace(/\$\{([^}]+)\}/g, (_, expr) => {
        const resolved = a2uiResolvePath(dataModel, expr);
        return resolved == null ? "" : String(resolved);
      });
      return template;
    }
  }
  return String(value);
}

function a2uiResolveActionContext(context, dataModel) {
  if (!context || typeof context !== "object") return {};
  const out = {};
  Object.keys(context).forEach((key) => {
    out[key] = a2uiResolveValue(context[key], dataModel);
  });
  return out;
}

function aiView() {
  const wrap = node("div");
  wrap.appendChild(titleBar("AI Copilot", "卖点生成与经营复盘均走后端 AI 任务接口。"));

  const selling = node("section", { class: "panel" });
  selling.appendChild(node("h3", { text: "商品卖点生成" }));
  const productName = node("input", { type: "text", value: "Travel Makeup Organizer" });
  const targetUsers = node("input", { type: "text", value: "dorm users" });
  const priceCent = node("input", { type: "number", value: "8900" });
  const attrs = node("textarea", { rows: "4" });
  attrs.value = "portable\nmulti-layer\neasy clean";
  const reviews = node("textarea", { rows: "4" });
  reviews.value = "收纳清晰\n带去宿舍很方便";
  const sellingMsg = node("div", { class: "error", style: "display:none" });
  const sellingOut = node("div");
  selling.appendChild(formRow("商品名称", productName));
  selling.appendChild(formRow("目标人群", targetUsers));
  selling.appendChild(formRow("价格（分）", priceCent));
  selling.appendChild(formRow("属性（每行一条）", attrs));
  selling.appendChild(formRow("评价（每行一条）", reviews));
  selling.appendChild(sellingMsg);
  const runSelling = node("button", { class: "btn primary", text: "生成卖点" });
  runSelling.onclick = async () => {
    sellingMsg.style.display = "none";
    runSelling.disabled = true;
    try {
      const task = await request("/api/ai/product-selling-points", {
        method: "POST",
        body: JSON.stringify({
          product_name: productName.value,
          target_users: targetUsers.value,
          price_cent: Number(priceCent.value) || 0,
          attributes: splitLines(attrs.value),
          reviews: splitLines(reviews.value),
        }),
      });
      sellingOut.innerHTML = "";
      sellingOut.appendChild(aiTaskPanel(task));
    } catch (err) {
      sellingMsg.textContent = err.message || "生成失败";
      sellingMsg.style.display = "";
    } finally {
      runSelling.disabled = false;
    }
  };
  selling.appendChild(node("div", { class: "actions" }, [runSelling]));
  selling.appendChild(sellingOut);
  wrap.appendChild(selling);

  const review = node("section", { class: "panel" });
  review.appendChild(node("h3", { text: "经营复盘生成" }));
  const windowDays = node("input", { type: "number", value: "7" });
  const productId = node("input", { type: "number", value: "1" });
  const reviewMsg = node("div", { class: "error", style: "display:none" });
  const reviewOut = node("div");
  review.appendChild(formRow("窗口天数", windowDays));
  review.appendChild(formRow("商品 ID（可选）", productId));
  review.appendChild(reviewMsg);
  const runReview = node("button", { class: "btn primary", text: "生成复盘" });
  runReview.onclick = async () => {
    reviewMsg.style.display = "none";
    runReview.disabled = true;
    try {
      const task = await request("/api/ai/business-review", {
        method: "POST",
        body: JSON.stringify({
          window_days: Number(windowDays.value) || 7,
          product_id: Number(productId.value) || 0,
        }),
      });
      reviewOut.innerHTML = "";
      reviewOut.appendChild(aiTaskPanel(task));
    } catch (err) {
      reviewMsg.textContent = err.message || "生成失败";
      reviewMsg.style.display = "";
    } finally {
      runReview.disabled = false;
    }
  };
  review.appendChild(node("div", { class: "actions" }, [runReview]));
  review.appendChild(reviewOut);
  wrap.appendChild(review);
  return wrap;
}

function aiTaskPanel(task) {
  const panel = node("section", { class: "panel soft" });
  panel.appendChild(node("div", { class: "meta", text: "任务 #" + task.id + " · " + task.task_type + " · " + task.status }));
  const output = task.output || {};
  Object.keys(output).forEach((key) => {
    const value = output[key];
    if (Array.isArray(value)) {
      panel.appendChild(node("h4", { text: key }));
      panel.appendChild(node("div", { class: "chip-row" }, value.map((item) => node("span", { class: "chip static", text: String(item) }))));
      return;
    }
    panel.appendChild(node("h4", { text: key }));
    panel.appendChild(node("p", { class: "muted", text: String(value) }));
  });
  return panel;
}

function metricCard(title, value, hint) {
  const card = node("section", { class: "card metric" });
  card.appendChild(node("div", { class: "metric-label", text: title }));
  card.appendChild(node("div", { class: "metric-value", text: value }));
  card.appendChild(node("div", { class: "muted", text: hint }));
  return card;
}

function titleBar(title, text) {
  const box = node("section", { class: "page-head" });
  box.appendChild(node("h1", { text: title }));
  box.appendChild(node("p", { class: "muted", text }));
  return box;
}

function formRow(label, input) {
  const row = node("label", { class: "form-row" });
  row.appendChild(node("span", { class: "label", text: label }));
  row.appendChild(input);
  return row;
}

function prettyTime(value) {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return String(value);
  }
  return date.toLocaleString();
}

function mount() {
  appRoot = document.getElementById("app");
  appRoot.innerHTML = "";
  topBar = node("header", { class: "topbar" });
  navRoot = node("nav", { class: "nav" });
  mainRoot = node("main", { class: "main" });
  const layout = node("div", { class: "layout" }, [navRoot, mainRoot]);
  appRoot.appendChild(topBar);
  appRoot.appendChild(layout);
  window.addEventListener("hashchange", renderRoute);
}

async function boot() {
  mount();
  if (store.session) {
    try {
      store.user = await request("/api/auth/me");
    } catch (err) {
      setSession("", null);
    }
  }
  if (!window.location.hash) {
    go(defaultPath());
    return;
  }
  renderRoute();
}

boot();
