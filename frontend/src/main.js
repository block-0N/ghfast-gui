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
        if (text.includes('release-asset')) {
            downloadBtn.style.display = 'inline-block';
        }
    } catch (err) {
        result.textContent = '错误: ' + err;
    }
});

downloadBtn.addEventListener('click', async () => {
    downloadBtn.disabled = true;
    progressWrap.style.display = 'block';
    try {
        await Download(currentURL);
    } catch (err) {
        result.textContent = '下载错误: ' + err;
    } finally {
        downloadBtn.disabled = false;
    }
});

EventsOn('download:progress', (data) => {
    const pct = data.total > 0 ? (data.downloaded / data.total * 100) : 0;
    progressBar.style.width = pct.toFixed(1) + '%';
    const mb = (data.downloaded / 1024 / 1024).toFixed(1);
    const totalMB = (data.total / 1024 / 1024).toFixed(1);
    const speed = (data.speed / 1024 / 1024).toFixed(2);
    if (data.done) {
        progressText.textContent = `完成 · 保存到 ${data.file}`;
    } else {
        progressText.textContent = `${pct.toFixed(1)}%  ${mb}/${totalMB} MB  ${speed} MB/s`;
    }
});