import './style.css';
import './app.css';

import { Parse, Download } from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime/runtime';

const input = document.getElementById('url-input');
const parseBtn = document.getElementById('parse-btn');
const downloadBtn = document.getElementById('download-btn');
const result = document.getElementById('result');
const progressWrap = document.getElementById('progress-wrap');
const progressBar = document.getElementById('progress-bar');
const progressText = document.getElementById('progress-text');

let currentURL = '';

parseBtn.addEventListener('click', async () => {
    result.textContent = '解析中...';
    downloadBtn.style.display = 'none';
    progressWrap.style.display = 'none';
    try {
        const text = await Parse(input.value);
        result.textContent = text;
        currentURL = input.value.trim();
        downloadBtn.style.display = 'inline-block';
        downloadBtn.textContent = '下载';
        downloadBtn.disabled = false;
    } catch (err) {
        result.textContent = '错误: ' + err;
    }
});

downloadBtn.addEventListener('click', async () => {
    downloadBtn.disabled = true;
    downloadBtn.textContent = '下载中...';
    progressWrap.style.display = 'block';
    progressBar.style.width = '0%';
    progressText.textContent = '准备中...';
    try {
        await Download(currentURL);
        downloadBtn.textContent = '完成';
    } catch (err) {
        result.textContent = '下载错误: ' + err;
        downloadBtn.textContent = '重试';
        downloadBtn.disabled = false;
    }
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
});