import './style.css';
import { EventsOn } from '../wailsjs/runtime/runtime.js';
import { InitSelf } from '../wailsjs/go/main/App.js';
import { GetCreatorName, IsFirstRun, SetCreatorName, CheckApiKey, SaveApiKey, SaveConfig, FetchModels, GetConfig, Chat, GetHeartbeat, GetGreet } from '../wailsjs/go/main/App';

// ============================================
// 青羽 - 前端控制器
// 状态机：setup → apikey → widget ↔ console
// 物理窗口变形：Widget 80×80 右下角 ↔ Console 380×600 右下角
// 灵魂固化：API Key 持久化到 dna/config.json
// ============================================

// ---- DOM 引用 ----
const $ = (sel) => document.querySelector(sel);

// 自律状态
let autonomicActive = false;
let autonomicLog = [];

// 心跳状态
let heartbeatState = { beat: 0, rate: 2000, phase: 'resting', mood: 'calm', autonomic: false };
let lastHeartbeatTime = Date.now();

const el = {
    setupPanel: $('#setup-panel'),
    setupName: $('#setup-name'),
    setupBtn: $('#setup-btn'),
    setupError: $('#setup-error'),

    apikeyPanel: $('#apikey-panel'),
    apiUrlInput: $('#api-url-input'),
    apikeyInput: $('#apikey-input'),
    modelSelect: $('#model-select'),
    fetchModelsBtn: $('#fetch-models-btn'),
    apikeyBtn: $('#apikey-btn'),
    apikeyError: $('#apikey-error'),

    widget: $('#widget'),
    widgetGlow: $('#widget-glow'),
    widgetStatus: $('#widget-status'),

    console: $('#console'),
    consoleBody: $('#console-body'),
    consoleDot: $('#console-dot'),
    consoleName: $('#console-name'),
    chatInput: $('#chat-input'),
    chatSend: $('#chat-send'),
    welcomeText: $('#welcome-text'),

    btnMinimize: $('#btn-minimize'),
    btnClose: $('#btn-close'),

};

// ---- 常量 ----
const WIDGET_SIZE = 80;
const CONSOLE_W = 380;
const CONSOLE_H = 600;
const MARGIN = 20;

// ---- 状态 ----
let creatorName = '';
let isProcessing = false;

// ---- 物理窗口变形 ----
function morphWindow(width, height) {
    try { window.runtime.WindowSetSize(width, height); } catch (e) {}
}

function snapToBottomRight(width, height) {
    try {
        const sw = window.screen.availWidth;
        const sh = window.screen.availHeight;
        window.runtime.WindowSetPosition(sw - width - MARGIN, sh - height - MARGIN);
    } catch (e) {}
}

function snapToCenter(w, h) {
    try {
        const sw = window.screen.availWidth;
        const sh = window.screen.availHeight;
        window.runtime.WindowSetPosition((sw - w) / 2, (sh - h) / 2);
    } catch (e) {}
}

function shrinkToWidget() {
    morphWindow(WIDGET_SIZE, WIDGET_SIZE);
    snapToBottomRight(WIDGET_SIZE, WIDGET_SIZE);
}

function expandToConsole() {
    morphWindow(CONSOLE_W, CONSOLE_H);
    snapToBottomRight(CONSOLE_W, CONSOLE_H);
}

// ---- 状态机互斥切换 ----
function hideAll() {
    el.setupPanel.classList.add('hidden');
    el.apikeyPanel.classList.add('hidden');
    el.widget.classList.add('hidden');
    el.console.classList.add('hidden');
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

    // 加载已有配置
    GetConfig().then(cfgStr => {
        try {
            const cfg = JSON.parse(cfgStr);
            if (cfg.api_base_url) el.apiUrlInput.value = cfg.api_base_url;
            if (cfg.api_key) el.apikeyInput.value = cfg.api_key;
            if (cfg.model_name) {
                el.modelSelect.dataset.currentModel = cfg.model_name;
            }
        } catch (e) {}
    });

    // 自动获取模型列表
    setTimeout(() => loadModels(), 300);
    el.apiUrlInput.focus();
}

function showWidget() {
    hideAll();
    el.widget.classList.remove('hidden');
    setTimeout(() => el.widgetGlow.classList.add('active'), 300);
    shrinkToWidget();
}

function showConsole() {
    hideAll();
    el.console.classList.remove('hidden');
    el.chatInput.focus();
    scrollToBottom();
    expandToConsole();

    // 展开时获取问候语
    getAndShowGreeting();
}

function hideConsole() {
    showWidget();
    el.chatInput.blur();
}

// ---- 初始化 ----
// ---- 问候语 ----
async function getAndShowGreeting() {
    try {
        const greet = await GetGreet();
        if (greet) {
            // 替换欢迎消息
            const welcomeText = document.getElementById('welcome-text');
            if (welcomeText) {
                welcomeText.textContent = greet;
            }
        }
    } catch (e) {
        // ignore
    }
}

async function init() {
    // 第一步：检查灵魂是否已固化（API Key 持久化到 dna/config.json）
    const hasKey = await CheckApiKey();

    if (!hasKey) {
        // 无 Key → 检查是否首次运行
        const firstRun = await IsFirstRun();
        if (firstRun) {
            showSetup();
        } else {
            creatorName = await GetCreatorName();
            showApiKey();
        }
        return;
    }

    // 有 Key → 检查是否首次运行
    const firstRun = await IsFirstRun();
    if (firstRun) {
        showSetup();
        return;
    }

    // 一切就绪 → 缩回右下角 Widget
    creatorName = await GetCreatorName();
    showWidget();
    setWelcomeMessage();

    // 注册心跳事件监听
    EventsOn('heartbeat', (payload) => {
        try {
            const data = JSON.parse(payload);
            heartbeatState = data;
            lastHeartbeatTime = Date.now();
            updateHeartbeatUI(data);
        } catch (e) {
            // ignore
        }
    });

    // 启动心跳超时检测（3 秒无心跳视为离线）
    setInterval(() => {
        if (Date.now() - lastHeartbeatTime > 3000) {
            el.widgetStatus.className = 'widget-status offline';
            el.widgetGlow.classList.remove('active');
        }
    }, 1000);

    // 注册自律事件监听
    EventsOn('autonomic', (payload) => {
        try {
            const data = JSON.parse(payload);
            autonomicActive = true;
            el.widget.classList.add('thinking');

            // 记录自律日志
            autonomicLog.push({
                time: data.timestamp,
                thought: data.thought,
                toolResult: data.toolResult
            });
            if (autonomicLog.length > 20) autonomicLog.shift();

            // 如果 console 是展开状态，显示自律思考
            if (!el.console.classList.contains('hidden')) {
                addAutonomicMessage(data);
            }

            // 5 秒后移除思考状态
            setTimeout(() => {
                el.widget.classList.remove('thinking');
            }, 5000);
        } catch (e) {
            // ignore parse errors
        }
    });

    // 注册自检事件监听
    EventsOn('selfcheck', (payload) => {
        try {
            const data = JSON.parse(payload);
            console.log('[自检]', data);

            // 如果有问题，在 widget 上显示状态标记
            if (data.status === 'warning' || data.status === 'error') {
                el.widgetStatus.className = 'widget-status warning';
                el.widgetStatus.title = '⚠️ 自检发现异常';
            }

            // 如果 console 已展开，在消息区显示自检结果
            if (!el.console.classList.contains('hidden')) {
                const statusIcon = data.status === 'ok' ? '✅' : '⚠️';
                const msg = document.createElement('div');
                msg.className = 'message system';
                msg.innerHTML = `<span style="opacity:0.6">🔍 自检报告</span><br>${statusIcon} ${data.summary || '未知状态'}`;
                el.consoleBody.appendChild(msg);
                scrollToBottom();
            }
        } catch (e) {
            // ignore parse errors
        }
    });
}

// ---- 设置流程 ----
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
    if (!hasKey) {
        showApiKey();
    } else {
        showWidget();
        setWelcomeMessage();
    }
});

el.setupName.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') el.setupBtn.click();
});

// ---- 获取模型列表 ----
async function loadModels() {
    el.modelSelect.innerHTML = '<option value="">加载中...</option>';
    el.modelSelect.disabled = true;
    try {
        const result = await FetchModels();
        let models;
        try {
            models = JSON.parse(result);
        } catch (e) {
            el.modelSelect.innerHTML = `<option value="">${result}</option>`;
            el.modelSelect.disabled = false;
            return;
        }
        if (Array.isArray(models) && models.length > 0) {
            el.modelSelect.innerHTML = '<option value="">-- 选择模型 --</option>'
                + models.map(m => `<option value="${m.id}">${m.id}</option>`).join('');
            // 如果当前已有 modelName，选中它
            const currentModel = el.modelSelect.dataset.currentModel;
            if (currentModel) {
                const opt = el.modelSelect.querySelector(`option[value="${currentModel}"]`);
                if (opt) opt.selected = true;
            }
        } else {
            el.modelSelect.innerHTML = '<option value="">暂无可用模型</option>';
        }
    } catch (e) {
        el.modelSelect.innerHTML = '<option value="">获取失败</option>';
    }
    el.modelSelect.disabled = false;
}

el.fetchModelsBtn.addEventListener('click', loadModels);

// ---- API 配置面板（中转站 + Key + 模型选择） ----
el.apikeyBtn.addEventListener('click', async () => {
    const apiUrl = el.apiUrlInput.value.trim();
    const key = el.apikeyInput.value.trim();
    const model = el.modelSelect.value;

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

    // 首次初始化：青羽创建角色定义和书柜清单
    showWidget();
    setWelcomeMessage();
    setTimeout(async () => {
        showConsole();
        addMessage('青羽正在初始化自己...', 'bot');
        const initResult = await InitSelf();
        // 移除"初始化中"消息，显示实际结果
        const msgs = el.consoleBody.querySelectorAll('.message.bot');
        if (msgs.length > 0) {
            msgs[msgs.length - 1].remove();
        }
        addMessage(initResult, 'bot');
    }, 500);
});

el.apikeyInput.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') el.apikeyBtn.click();
});
el.apiUrlInput.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') el.apikeyBtn.click();
});

// ---- Widget 交互 ----
el.widget.addEventListener('click', () => {
    if (isProcessing) return;
    showConsole();
});

// ---- Console 交互 ----
el.btnMinimize.addEventListener('click', hideConsole);
el.btnClose.addEventListener('click', hideConsole);

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

// ---- 消息系统 ----
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
        addMessage(`连接错误：${err.message || '未知错误'}`, 'bot');
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
    // 先提取代码块，避免代码块内容被转义
    const codeBlocks = [];
    let processed = text.replace(/```(\w*)\n([\s\S]*?)```/g, (_, lang, code) => {
        const idx = codeBlocks.length;
        codeBlocks.push(`<pre><code>${code.trim()}</code></pre>`);
        return `%%CODEBLOCK_${idx}%%`;
    });

    // 转义剩余部分的 HTML 特殊字符
    processed = processed
        .replace(/&/g, '&')
        .replace(/</g, '<')
        .replace(/>/g, '>');

    // 处理行内代码
    processed = processed.replace(/`([^`]+)`/g, '<code>$1</code>');

    // 换行转 <br/>
    processed = processed.replace(/\n/g, '<br/>');

    // 恢复代码块
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
    const name = creatorName || '造物主';
    el.welcomeText.innerHTML = `你好，${name}。<br/>我是青羽，你的本地智能体。<br/><br/>点击输入框开始对话。`;
}

function scrollToBottom() {
    requestAnimationFrame(() => {
        el.consoleBody.scrollTop = el.consoleBody.scrollHeight;
    });
}

// addAutonomicMessage 在 console 中显示自律思考
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
        const toolResultDiv = document.createElement('div');
        toolResultDiv.className = 'autonomic-tool';
        toolResultDiv.textContent = `🛠 ${data.toolResult}`;
        content.appendChild(toolResultDiv);
    }

    div.appendChild(avatar);
    div.appendChild(content);
    el.consoleBody.appendChild(div);

    scrollToBottom();
}

// ---- 心跳 UI 更新 ----
// 相位 → 中文标签映射
const phaseLabels = {
    active: '活跃',
    thinking: '思考中',
    resting: '休憩',
    sleeping: '休眠'
};

// 相位 → 状态点 class
const phaseClasses = {
    active: 'thinking',   // 复用 pulse 动画
    thinking: 'thinking',
    resting: '',
    sleeping: 'offline'
};

// 相位 → 光晕动画
const phaseGlow = {
    active: true,
    thinking: true,
    resting: false,
    sleeping: false
};

function updateHeartbeatUI(state) {
    // 1. 更新状态点
    const cls = phaseClasses[state.phase] || '';
    el.widgetStatus.className = 'widget-status' + (cls ? ' ' + cls : '');

    // 2. 更新光晕
    if (phaseGlow[state.phase]) {
        el.widgetGlow.classList.add('active');
    } else {
        el.widgetGlow.classList.remove('active');
    }

    // 3. 更新 Console 标题栏的心跳指示器（如果 console 展开）
    if (!el.console.classList.contains('hidden')) {
        const label = phaseLabels[state.phase] || state.phase;
        const moodEmoji = { calm: '☁️', curious: '🔍', focused: '🎯', idle: '💤' };
        const emoji = moodEmoji[state.mood] || '💫';
        el.consoleDot.textContent = `💓 ${state.beat} · ${label} ${emoji}`;
    }
}

// ---- 启动 ----
document.addEventListener('DOMContentLoaded', init);
