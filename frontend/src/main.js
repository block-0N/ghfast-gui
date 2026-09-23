import './style.css';
import './app.css';

import { Parse, Download, Cancel } from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime/runtime';

const input = document.getElementById('url-input');
const parseBtn = document.getElementById('parse-btn');
const downloadBtn = document.getElementById('download-btn');
const cancelBtn = document.getElementById('cancel-btn');
const result = document.getElementById('result');
const progressWrap = document.getElementById('progress-wrap');
const progressBar = document.getElementById('progress-bar');
const progressText = document.getElementById('progress-text');
const historySection = document.getElementById('history-section');
const historyList = document.getElementById('history-list');

let currentURL = '';
let downloading = false;

const HISTORY_KEY = 'ghfast_history';

function loadHistory() {
    try {
        return JSON.parse(localStorage.getItem(HISTORY_KEY) || '[]');
    } catch {
        return [];
    }
}

function saveHistory(list) {
    localStorage.setItem(HISTORY_KEY, JSON.stringify(list.slice(0, 20)));
}

function renderHistory() {
    const list = loadHistory();
    if (list.length === 0) {
        historySection.style.display = 'none';
        return;
    }
    historySection.style.display = 'block';
    historyList.innerHTML = '';
    list.forEach((item, idx) => {
        const li = document.createElement('li');
        li.className = 'history-item';

        const time = new Date(item.time).toLocaleString('zh-CN', {
            month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit',
        });

        li.innerHTML = `
      <span class="hist-name">${item.filename || item.url.slice(0, 50)}</span>
      <span class="hist-time">${time}</span>
      <span class="hist-size">${item.size || ''}</span>
    `;
        li.addEventListener('click', () => {
            input.value = item.url;
            parseBtn.click();
        });
        historyList.appendChild(li);
    });
}

function addHistory(url, filename, size) {
    const list = loadHistory();
    const filtered = list.filter((x) => x.url !== url);
    filtered.unshift({ url, filename, size, time: Date.now() });
    saveHistory(filtered);
    renderHistory();
}

parseBtn.addEventListener('click', async () => {
    result.textContent = '解析中...';
    downloadBtn.style.display = 'none';
    cancelBtn.style.display = 'none';
    progressWrap.style.display = 'none';
    try {
        const text = await Parse(input.value);
        result.textContent = text;
        currentURL = input.value.trim();

        const filenameMatch = text.match(/文件:\s*(.+)/);
        const filename = filenameMatch ? filenameMatch[1].trim() : '';

        downloadBtn.style.display = 'inline-block';
        downloadBtn.textContent = '下载';
        downloadBtn.disabled = false;
        downloadBtn.dataset.filename = filename;
    } catch (err) {
        result.textContent = '错误: ' + err;
    }
});

downloadBtn.addEventListener('click', async () => {
    downloading = true;
    downloadBtn.disabled = true;
    downloadBtn.textContent = '下载中...';
    cancelBtn.style.display = 'inline-block';
    cancelBtn.disabled = false;
    progressWrap.style.display = 'block';
    progressBar.style.width = '0%';
    progressText.textContent = '准备中...';

    try {
        await Download(currentURL);
        downloadBtn.textContent = '完成';
        addHistory(currentURL, downloadBtn.dataset.filename || '', '');
    } catch (err) {
        const msg = String(err);
        if (msg.includes('context canceled')) {
            downloadBtn.textContent = '已取消';
            progressText.textContent = '已取消（分片保留，重新点击可从断点继续）';
        } else {
            result.textContent = '下载错误: ' + msg;
            downloadBtn.textContent = '重试';
            downloadBtn.disabled = false;
        }
    } finally {
        downloading = false;
        cancelBtn.style.display = 'none';
    }
});

cancelBtn.addEventListener('click', async () => {
    cancelBtn.disabled = true;
    cancelBtn.textContent = '取消中...';
    await Cancel();
    cancelBtn.textContent = '取消';
});

EventsOn('download:file', (d) => {
    progressText.textContent = `正在下载 (${d.index + 1}/${d.total}): ${d.name}`;
    progressBar.style.width = '0%';
});

EventsOn('download:progress', (d) => {
    const pct = d.total > 0 ? (d.downloaded / d.total * 100) : 0;
    progressBar.style.width = pct.toFixed(1) + '%';
    const mb = (d.downloaded / 1024 / 1024).toFixed(1);
    const totalMB = d.total > 0 ? (d.total / 1024 / 1024).toFixed(1) : '?';
    const speed = (d.speed / 1024 / 1024).toFixed(2);
    if (d.done) {
        progressText.textContent = `完成 · ${d.file}`;
    } else {
        progressText.textContent = `${pct.toFixed(1)}%  ${mb}/${totalMB} MB  ${speed} MB/s`;
    }
});

EventsOn('download:done', (d) => {
    progressText.textContent = `全部完成 · 保存于 ${d.dir}`;
    downloadBtn.textContent = '完成';
    cancelBtn.style.display = 'none';
});

renderHistory();