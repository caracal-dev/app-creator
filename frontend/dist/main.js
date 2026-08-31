const state = {
  packagePath: "",
  iconPath: "",
  iconBase64: "",
  appName: "",
  running: false,
  defaultIcon: "",
};

const elements = {
  statusPill: document.querySelector("#status-pill"),
  pickFileButton: document.querySelector("#pick-file-button"),
  selectedFile: document.querySelector("#selected-file"),
  convertButton: document.querySelector("#convert-button"),
  pickIconButton: document.querySelector("#pick-icon-button"),
  iconPreview: document.querySelector("#icon-preview"),
  iconImage: document.querySelector("#icon-image"),
  iconLabel: document.querySelector("#icon-label"),
  appNameInput: document.querySelector("#app-name-input"),
  progressSection: document.querySelector("#progress-section"),
  progressState: document.querySelector("#progress-state"),
  progressDetail: document.querySelector("#progress-detail"),
  resultSection: document.querySelector("#result-section"),
  resultBody: document.querySelector("#result-body"),
  errorSection: document.querySelector("#error-section"),
  errorDetail: document.querySelector("#error-detail"),
  clearButton: document.querySelector("#clear-button"),
  retryButton: document.querySelector("#retry-button"),
};

function backend() {
  const bound = window.go?.guiapp?.App || window.go?.main?.App;
  if (!bound) {
    throw new Error("Wails backend bindings are not available.");
  }
  return bound;
}

async function boot() {
  bindEvents();
  bindRuntimeEvents();
  state.defaultIcon = await backend().DefaultAppIcon();
  renderPreview(state.defaultIcon, false);
  setStatus("ready", "neutral");
}

function bindEvents() {
  elements.pickFileButton.addEventListener("click", pickFile);
  elements.pickIconButton.addEventListener("click", pickIcon);
  elements.iconPreview.addEventListener("click", pickIcon);
  elements.appNameInput.addEventListener("input", () => {
    state.appName = elements.appNameInput.value.trim();
    updateConvertButton();
  });
  elements.convertButton.addEventListener("click", convertPackage);
  elements.clearButton.addEventListener("click", resetAll);
  elements.retryButton.addEventListener("click", () => {
    hide(elements.errorSection);
    show(elements.progressSection);
    convertPackage();
  });
}

function bindRuntimeEvents() {
  if (!window.runtime?.EventsOn) return;

  window.runtime.EventsOn("creator:phase", (payload) => {
    if (payload.state === "running") {
      elements.progressDetail.textContent = payload.message;
      setStatus(payload.state, "running");
    } else if (payload.state === "complete") {
      elements.progressDetail.textContent = payload.message;
      setStatus("complete", "success");
    }
  });
}

async function pickFile() {
  try {
    const path = await backend().PickFile();
    if (!path) return;
    state.packagePath = path;
    elements.selectedFile.textContent = path;
    elements.selectedFile.classList.add("is-set");
    updateConvertButton();
  } catch (err) {
    console.error("pick file failed", err);
  }
}

async function pickIcon() {
  try {
    const path = await backend().PickIcon();
    if (!path) return;
    state.iconPath = path;
    // Read the icon as base64 for preview
    try {
      const b64 = await backend().ReadFileAsBase64(path);
      state.iconBase64 = b64;
      renderPreview(b64, true);
    } catch (_) {
      renderPreview(state.defaultIcon, false);
    }
  } catch (err) {
    console.error("pick icon failed", err);
  }
}

function renderPreview(b64, isCustom) {
  if (b64) {
    elements.iconImage.src = "data:image/png;base64," + b64;
    elements.iconImage.classList.add("is-visible");
    elements.iconLabel.textContent = isCustom ? "Custom icon" : "Default icon";
  } else {
    elements.iconImage.classList.remove("is-visible");
    elements.iconLabel.textContent = "No icon";
  }
}

async function convertPackage() {
  if (state.running) return;

  state.running = true;
  elements.convertButton.disabled = true;
  show(elements.progressSection);
  hide(elements.resultSection);
  hide(elements.errorSection);

  elements.progressState.textContent = "Converting";
  setStatus("running", "running");

  try {
    const result = await backend().ConvertPackage({
      packagePath: state.packagePath,
      iconPath: state.iconPath,
      appName: state.appName,
    });

    if (!result.success) {
      showError(result.error);
      return;
    }

    renderResult(result);
    setStatus("complete", "success");
  } catch (err) {
    showError(err?.message || String(err));
  } finally {
    state.running = false;
    elements.convertButton.disabled = false;
  }
}

function renderResult(result) {
  hide(elements.progressSection);
  show(elements.resultSection);

  const body = elements.resultBody;
  body.innerHTML = "";

  // Icon
  const iconHtml = result.iconBase64
    ? `<img class="result-icon" src="data:image/png;base64,${result.iconBase64}" alt="App icon" />`
    : `<div class="result-icon" style="display:flex;align-items:center;justify-content:center;color:var(--muted);font-size:0.78rem;">No icon</div>`;

  body.innerHTML = `
    <div style="display:flex;gap:16px;align-items:center;">
      ${iconHtml}
      <div class="result-detail">
        <strong>${escHtml(result.detectedName)}</strong>
        <p>AppImage created successfully</p>
      </div>
    </div>
    <p class="result-path">${escHtml(result.appImagePath)}</p>
  `;

  elements.statusPill.textContent = "Done";
  elements.statusPill.className = "status-badge success";
}

function showError(msg) {
  hide(elements.progressSection);
  show(elements.errorSection);
  elements.errorDetail.textContent = msg;
  elements.statusPill.textContent = "Failed";
  elements.statusPill.className = "status-badge error";
}

function resetAll() {
  state.packagePath = "";
  state.iconPath = "";
  state.iconBase64 = "";
  state.appName = "";
  state.running = false;

  elements.selectedFile.textContent = "No file selected";
  elements.selectedFile.classList.remove("is-set");
  elements.appNameInput.value = "";
  elements.convertButton.disabled = true;

  hide(elements.progressSection);
  hide(elements.resultSection);
  hide(elements.errorSection);

  renderPreview(state.defaultIcon, false);
  setStatus("ready", "neutral");
}

function updateConvertButton() {
  elements.convertButton.disabled = !state.packagePath;
}

function setStatus(label, className) {
  elements.statusPill.textContent = label;
  elements.statusPill.className = "status-badge " + className;
}

function show(el) { el.classList.remove("is-hidden"); }
function hide(el) { el.classList.add("is-hidden"); }

function escHtml(s) {
  const d = document.createElement("div");
  d.appendChild(document.createTextNode(s || ""));
  return d.innerHTML;
}

// Boot on DOM load
document.addEventListener("DOMContentLoaded", boot);