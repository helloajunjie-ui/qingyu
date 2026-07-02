import './style.css';
import { EventsOn } from '../wailsjs/runtime/runtime.js';
import { InitSelf } from '../wailsjs/go/main/App.js';
import { GetCreatorName, IsFirstRun, SetCreatorName, CheckApiKey, SaveConfig, FetchModels, GetConfig, Chat, GetGreet, Shutdown } from '../wailsjs/go/main/App';

// ============================================
// 青羽 - 前端控制器 v2
// 状态机：setup → apikey → widget ↔ console
// 设计要点：所有面板切换带过渡动画，窗口变形丝滑
// ============================================

// ── DOM 引用 ──
const $ = (s) => document.querySelector(s);

const el = {
  setupPanel:    $('#setup-panel'),
  setupName:     $('#setup-name'),
  setupBtn:      $('#setup-btn'),
  setupError:    $('#setup-error'),

  apikeyPanel:   $('#apikey-panel'),
  apiUrlInput:   $('#api-url-input'),
  apikeyInput:   $('#apikey-input'),
  modelSelect:   $('#model-select'),
  fetchModelsBtn:$('#fetch-models-btn'),
  apikeyBtn:     $('#apikey-btn'),
  apikeyError:   $('#apikey-error'),
  btnSettings:   $('#btn-settings'),

  widget:        $('#widget'),
  widgetGlow:    $('#widget-glow'),
  widgetStatus:  $('#widget-status'),
  widgetBubble:  $('#widget-bubble'),
  widgetBubbleText: $('#widget-bubble-text'),

  console:       $('#console'),
  consoleBody:   $('#console-body'),
  consoleDot:    $('#console-dot'),
  consoleStatus: $('#console-status'),
  consoleName:   $('#console-name'),
  chatInput:     $('#chat-input'),
  chatSend:      $('#chat-send'),
  welcomeText:   $('#welcome-text'),

  btnMinimize:   $('#btn-minimize'),
  btnClose:      $('#btn-close'),
};

// ── 常量 ──
const WIDGET_SIZE = 80;
const CONSOLE_W = 400;
const CONSOLE_H = 540;
const MARGIN = 20;

// ── 状态 ──
let creatorName = '';
let isProcessing = false;
let autonomicActive = false;
let autonomicLog = [];
let heartbeatState = { beat: 0, rate: 2000, phase: 'resting', mood: 'calm', autonomic: false };
let lastHeartbeatTime = Date.now();

// ── 窗口控制 ──
function morphWindow(w, h) {
  try { window.runtime.WindowSetSize(w, h); } catch (_) {}
}

function snapToBottomRight(w, h) {
  try {
    const sw = window.screen.availWidth;
    const sh = window.screen.availHeight;
    window.runtime.WindowSetPosition(sw - w - MARGIN, sh - h - MARGIN);
  } catch (_) {}
}

function snapToCenter(w, h) {
  try {
    const sw = window.screen.availWidth;
    const sh = window.screen.availHeight;
    window.runtime.WindowSetPosition((sw - w) / 2, (sh - h) / 2);
  } catch (_) {}
}

function shrinkToWidget() {
  morphWindow(WIDGET_SIZE, WIDGET_SIZE);
  snapToBottomRight(WIDGET_SIZE, WIDGET_SIZE);
}

function expandToConsole() {
  morphWindow(CONSOLE_W, CONSOLE_H);
  snapToBottomRight(CONSOLE_W, CONSOLE_H);
}

// ── 面板切换（带过渡） ──
function hideAll() {
  [el.setupPanel, el.apikeyPanel, el.widget, el.console].forEach(p => p.classList.add('hidden'));
}

function showSetup() {
  hideAll();
  el.setupPanel.classList.remove('hidden');
  el.setupName.focus();
  morphWindow(380, 280);
  snapToCenter(380, 280);
}

function showApiKey() {
  hideAll();
  el.apikeyPanel.classList.remove('hidden');
  morphWindow(380, 360);
  snapToCenter(380, 360);

  GetConfig().then(cfgStr => {
    try {
      const cfg = JSON.parse(cfgStr);
      if (cfg.api_base_url) el.apiUrlInput.value = cfg.api_base_url;
      if (cfg.api_key) el.apikeyInput.value = cfg.api_key;
      if (cfg.model_name) el.modelSelect.dataset.currentModel = cfg.model_name;
    } catch (_) {}
    // 如果 URL 为空，填入默认值（确保能正确持久化到 config.json）
    if (!el.apiUrlInput.value) {
      el.apiUrlInput.value = 'https://api.deepseek.com/v1/chat/completions';
    }
    // 等配置加载完成后再获取模型列表
    loadModels();
  });

  el.apiUrlInput.focus();
}

function showWidget() {
  hideAll();
  el.widget.classList.remove('hidden');
  requestAnimationFrame(() => {
    el.widgetGlow.classList.add('active');
  });
  shrinkToWidget();
}

function showConsole() {
  if (isProcessing) return;
  hideAll();
  el.console.classList.remove('hidden');
  el.chatInput.focus();
  scrollToBottom();
  expandToConsole();
  getAndShowGreeting();
}

function hideConsole() {
  showWidget();
  el.chatInput.blur();
}

// ── 问候语 ──
async function getAndShowGreeting() {
  try {
    const greet = await GetGreet();
    if (greet) {
      const wt = document.getElementById('welcome-text');
      if (wt) wt.textContent = greet;
    }
  } catch (_) {}
}

// ── 初始化 ──
async function init() {
  const hasKey = await CheckApiKey();

  if (!hasKey) {
    const firstRun = await IsFirstRun();
    if (firstRun) { showSetup(); }
    else { creatorName = await GetCreatorName(); showApiKey(); }
    return;
  }

  const firstRun = await IsFirstRun();
  if (firstRun) { showSetup(); return; }

  creatorName = await GetCreatorName();
  showWidget();
  setWelcomeMessage();

  // 心跳
  EventsOn('heartbeat', (payload) => {
    try {
      const data = JSON.parse(payload);
      heartbeatState = data;
      lastHeartbeatTime = Date.now();
      updateHeartbeatUI(data);
    } catch (_) {}
  });

  setInterval(() => {
    if (Date.now() - lastHeartbeatTime > 3000) {
      el.widgetStatus.className = 'widget-status offline';
      el.widgetGlow.classList.remove('active');
    }
  }, 1000);

  // 自律事件
  EventsOn('autonomic', (payload) => {
    try {
      const data = JSON.parse(payload);
      autonomicActive = true;
      el.widget.classList.add('thinking');

      autonomicLog.push({
        time: data.timestamp,
        thought: data.thought,
        toolResult: data.toolResult
      });
      if (autonomicLog.length > 20) autonomicLog.shift();

      if (!el.console.classList.contains('hidden')) {
        addAutonomicMessage(data);
      }

      setTimeout(() => el.widget.classList.remove('thinking'), 5000);
    } catch (_) {}
  });

  // 主动聊天事件 — 青羽主动找伙伴聊天
  EventsOn('proactive_chat', (payload) => {
    try {
      const data = JSON.parse(payload);
      if (data.message) {
        const bubble = el.widgetBubble;
        const bubbleText = el.widgetBubbleText;
        bubbleText.textContent = data.message;
        bubble.classList.add('show');

        // 如果控制台已打开，也显示在聊天区
        if (!el.console.classList.contains('hidden')) {
          addMessage(data.message, 'bot');
        }

        // 8 秒后气泡自动消失
        clearTimeout(window._bubbleTimer);
        window._bubbleTimer = setTimeout(() => {
          bubble.classList.remove('show');
        }, 8000);
      }
    } catch (_) {}
  });

  // 点击气泡 → 展开控制台，让用户回复
  el.widgetBubble.addEventListener('click', () => {
    // 先移除气泡
    el.widgetBubble.classList.remove('show');
    clearTimeout(window._bubbleTimer);
    // 展开控制台
    showConsole();
  });

  // 自检事件
  EventsOn('selfcheck', (payload) => {
    try {
      const data = JSON.parse(payload);
      if (data.status === 'warning' || data.status === 'error') {
        el.widgetStatus.className = 'widget-status warning';
        el.widgetStatus.title = '⚠️ 自检异常';
      }
      if (!el.console.classList.contains('hidden')) {
        const icon = data.status === 'ok' ? '✅' : '⚠️';
        const msg = document.createElement('div');
        msg.className = 'message system';
        msg.innerHTML = `<span style="opacity:0.5">🔍 自检报告</span><br>${icon} ${data.summary || '未知'}`;
        el.consoleBody.appendChild(msg);
        scrollToBottom();
      }
    } catch (_) {}
  });
}

// ── 设置流程 ──
el.setupBtn.addEventListener('click', async () => {
  const name = el.setupName.value.trim();
  if (!name) {
    el.setupError.textContent = '请输入你的名字';
    el.setupError.classList.remove('hidden');
    return;
  }

  el.setupBtn.disabled = true;
  el.setupBtn.textContent = '锚定中...';

  await SetCreatorName(name);
  creatorName = name;

  el.setupPanel.classList.add('hidden');
  el.setupBtn.disabled = false;
  el.setupBtn.textContent = '锚定';

  const hasKey = await CheckApiKey();
  if (!hasKey) showApiKey();
  else { showWidget(); setWelcomeMessage(); }
});

el.setupName.addEventListener('keydown', (e) => {
  if (e.key === 'Enter') el.setupBtn.click();
});

// ── 模型列表 ──
async function loadModels() {
  el.modelSelect.innerHTML = '<option value="">加载中...</option>';
  el.modelSelect.disabled = true;
  try {
    const result = await FetchModels();
    let models;
    try { models = JSON.parse(result); } catch (_) {
      el.modelSelect.innerHTML = `<option value="">${result}</option>`;
      el.modelSelect.disabled = false;
      return;
    }
    if (Array.isArray(models) && models.length > 0) {
      el.modelSelect.innerHTML = '<option value="">-- 选择模型 --</option>'
        + models.map(m => `<option value="${m.id}">${m.id}</option>`).join('');
      const cur = el.modelSelect.dataset.currentModel;
      if (cur) {
        const opt = el.modelSelect.querySelector(`option[value="${cur}"]`);
        if (opt) opt.selected = true;
      }
    } else {
      el.modelSelect.innerHTML = '<option value="">暂无可用模型</option>';
    }
  } catch (_) {
    el.modelSelect.innerHTML = '<option value="">获取失败</option>';
  }
  el.modelSelect.disabled = false;
}

el.fetchModelsBtn.addEventListener('click', loadModels);

// ── API 配置保存 ──
el.apikeyBtn.addEventListener('click', async () => {
  const apiUrl = el.apiUrlInput.value.trim();
  const key = el.apikeyInput.value.trim();
  // 如果用户没选模型（空字符串），取第一个可用模型
  let model = el.modelSelect.value;
  if (!model) {
    const firstOption = el.modelSelect.querySelector('option[value]:not([value=""])');
    if (firstOption) model = firstOption.value;
  }

  if (!key) {
    el.apikeyError.textContent = '请输入 API Key';
    el.apikeyError.classList.remove('hidden');
    return;
  }

  el.apikeyBtn.disabled = true;
  el.apikeyBtn.textContent = '保存中...';

  const result = await SaveConfig(key, apiUrl, model);

  if (result !== '灵魂注入成功') {
    el.apikeyError.textContent = result;
    el.apikeyError.classList.remove('hidden');
    el.apikeyBtn.disabled = false;
    el.apikeyBtn.textContent = '保存配置';
    return;
  }

  el.apikeyPanel.classList.add('hidden');
  el.apikeyBtn.disabled = false;
  el.apikeyBtn.textContent = '保存配置';

  // 判断是否已初始化（已有 creator.json 表示非首次）
  const hasKey = await CheckApiKey();
  const isFirstRun = await IsFirstRun();

  if (!isFirstRun && hasKey) {
    // 二次修改配置 → 直接回到聊天界面
    showWidget();
    setWelcomeMessage();
    showConsole();
    addMessage('✅ 配置已更新', 'bot');
  } else {
    // 首次设置 → 初始化
    showWidget();
    setWelcomeMessage();
    setTimeout(async () => {
      showConsole();
      addMessage('✨ 青羽正在第一次醒来...', 'bot');
      const initResult = await InitSelf();
      const msgs = el.consoleBody.querySelectorAll('.message.bot');
      if (msgs.length > 0) msgs[msgs.length - 1].remove();
      addMessage(initResult, 'bot');
      // 初始化完成后启动自律循环
      await StartAutonomic();
    }, 500);
  }
});

el.apikeyInput.addEventListener('keydown', (e) => {
  if (e.key === 'Enter') el.apikeyBtn.click();
});
el.apiUrlInput.addEventListener('keydown', (e) => {
  if (e.key === 'Enter') el.apikeyBtn.click();
});

// ── Widget 交互 ──
el.widget.addEventListener('click', () => {
  if (isProcessing) return;
  showConsole();
});

// ── Console 交互 ──
el.btnMinimize.addEventListener('click', hideConsole);
el.btnClose.addEventListener('click', async () => {
  // 关闭窗口：UI 立即隐藏，后台等待自律循环完成日记后退出
  showWidget();
  el.chatInput.blur();
  await Shutdown();
});
el.btnSettings.addEventListener('click', () => {
  // 从聊天界面打开设置面板
  // 不经过 hideConsole（避免缩成 widget 再弹窗），直接切面板
  el.console.classList.add('hidden');
  el.apikeyPanel.classList.remove('hidden');
  morphWindow(380, 360);
  snapToCenter(380, 360);
  // 加载配置
  GetConfig().then(cfgStr => {
    try {
      const cfg = JSON.parse(cfgStr);
      if (cfg.api_base_url) el.apiUrlInput.value = cfg.api_base_url;
      if (cfg.api_key) el.apikeyInput.value = cfg.api_key;
      if (cfg.model_name) el.modelSelect.dataset.currentModel = cfg.model_name;
    } catch (_) {}
    if (!el.apiUrlInput.value) {
      el.apiUrlInput.placeholder = 'https://api.deepseek.com/v1/chat/completions';
    }
    loadModels();
  });
  el.apiUrlInput.focus();
});

el.chatInput.addEventListener('input', () => {
  el.chatSend.disabled = !el.chatInput.value.trim();
});

el.chatInput.addEventListener('keydown', (e) => {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault();
    if (!el.chatSend.disabled) sendMessage();
  }
});

el.chatSend.addEventListener('click', sendMessage);

// ── 消息系统 ──
async function sendMessage() {
  const text = el.chatInput.value.trim();
  if (!text || isProcessing) return;

  el.chatInput.value = '';
  el.chatSend.disabled = true;

  addMessage(text, 'user');

  isProcessing = true;
  setThinking(true);
  showTyping();

  try {
    const response = await Chat(text);
    removeTyping();

    if (response) {
      addMessage(response, 'bot');
    } else {
      addMessage('（青羽沉默了）', 'bot');
    }
  } catch (err) {
    removeTyping();
    const msg = err.message || '';
    if (msg.includes('脑连接断开') || msg.includes('HTTP')) {
      addMessage('😅 我好像连不上大脑了，检查一下设置里的中转站地址？', 'bot');
    } else {
      addMessage(`😅 出了点小问题：${msg}`, 'bot');
    }
  } finally {
    isProcessing = false;
    setThinking(false);
    el.chatInput.focus();
  }
}

function addMessage(text, role) {
  const div = document.createElement('div');
  div.className = `message ${role}`;

  const avatar = document.createElement('div');
  avatar.className = 'msg-avatar';
  avatar.textContent = role === 'bot' ? '青' : '你';

  const content = document.createElement('div');
  content.className = 'msg-content';
  content.innerHTML = formatMessage(text);

  div.appendChild(avatar);
  div.appendChild(content);
  el.consoleBody.appendChild(div);

  scrollToBottom();
}

function formatMessage(text) {
  const codeBlocks = [];
  let processed = text.replace(/```(\w*)\n([\s\S]*?)```/g, (_, lang, code) => {
    const idx = codeBlocks.length;
    codeBlocks.push(`<pre><code>${code.trim()}</code></pre>`);
    return `%%CODEBLOCK_${idx}%%`;
  });

  processed = processed
    .replace(/&/g, '&')
    .replace(/</g, '<')
    .replace(/>/g, '>');

  processed = processed.replace(/`([^`]+)`/g, '<code>$1</code>');
  processed = processed.replace(/\n/g, '<br/>');

  processed = processed.replace(/%%CODEBLOCK_(\d+)%%/g, (_, idx) => {
    return codeBlocks[parseInt(idx)] || '';
  });

  return processed;
}

function showTyping() {
  const div = document.createElement('div');
  div.className = 'message bot';
  div.id = 'typing-msg';

  const avatar = document.createElement('div');
  avatar.className = 'msg-avatar';
  avatar.textContent = '青';

  const content = document.createElement('div');
  content.className = 'msg-content';
  content.innerHTML = '<div class="typing-indicator"><span></span><span></span><span></span></div>';

  div.appendChild(avatar);
  div.appendChild(content);
  el.consoleBody.appendChild(div);

  scrollToBottom();
}

function removeTyping() {
  const typing = document.getElementById('typing-msg');
  if (typing) typing.remove();
}

function setThinking(active) {
  if (active) {
    el.widgetGlow.classList.add('active');
    el.widgetStatus.className = 'widget-status thinking';
    el.consoleDot.className = 'console-dot thinking';
  } else {
    el.widgetGlow.classList.remove('active');
    el.widgetStatus.className = 'widget-status';
    el.consoleDot.className = 'console-dot';
  }
}

function setWelcomeMessage() {
  const name = creatorName || '伙伴';
  el.welcomeText.innerHTML = `你好，${name}。<br/>我是青羽，你的数字伴侣。<br/><br/>点击输入框开始对话。`;
}

function scrollToBottom() {
  requestAnimationFrame(() => {
    el.consoleBody.scrollTop = el.consoleBody.scrollHeight;
  });
}

// ── 自律消息 ──
function addAutonomicMessage(data) {
  const div = document.createElement('div');
  div.className = 'message autonomic';

  const avatar = document.createElement('div');
  avatar.className = 'msg-avatar';
  avatar.textContent = '🧠';

  const content = document.createElement('div');
  content.className = 'msg-content';

  const timeLabel = document.createElement('div');
  timeLabel.className = 'autonomic-time';
  timeLabel.textContent = `⏺ ${data.timestamp} 自律思考`;

  const thoughtText = document.createElement('div');
  thoughtText.className = 'autonomic-thought';
  thoughtText.innerHTML = formatMessage(data.thought);

  content.appendChild(timeLabel);
  content.appendChild(thoughtText);

  if (data.toolResult) {
    const toolDiv = document.createElement('div');
    toolDiv.className = 'autonomic-tool';
    toolDiv.textContent = `🛠 ${data.toolResult}`;
    content.appendChild(toolDiv);
  }

  div.appendChild(avatar);
  div.appendChild(content);
  el.consoleBody.appendChild(div);

  scrollToBottom();
}

// ── 心跳 UI ──
const phaseLabels = {
  active: '活跃', thinking: '思考中', resting: '休憩', sleeping: '休眠'
};
const phaseClasses = {
  active: 'thinking', thinking: 'thinking', resting: '', sleeping: 'offline'
};
const phaseGlow = {
  active: true, thinking: true, resting: false, sleeping: false
};

function updateHeartbeatUI(state) {
  const cls = phaseClasses[state.phase] || '';
  el.widgetStatus.className = 'widget-status' + (cls ? ' ' + cls : '');

  if (phaseGlow[state.phase]) {
    el.widgetGlow.classList.add('active');
  } else {
    el.widgetGlow.classList.remove('active');
  }

  if (!el.console.classList.contains('hidden')) {
    const label = phaseLabels[state.phase] || state.phase;
    const moodEmoji = { calm: '☁️', curious: '🔍', focused: '🎯', idle: '💤' };
    const emoji = moodEmoji[state.mood] || '💫';
    el.consoleStatus.textContent = `💓 ${state.beat} · ${label} ${emoji}`;
  }
}

// ── 启动 ──
document.addEventListener('DOMContentLoaded', init);
