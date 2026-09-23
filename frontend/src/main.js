import './style.css';
import './app.css';

import { Parse } from '../wailsjs/go/main/App';

const input = document.getElementById('url-input');
const btn = document.getElementById('parse-btn');
const result = document.getElementById('result');

btn.addEventListener('click', async () => {
    result.textContent = '解析中...';
    try {
        const text = await Parse(input.value);
        result.textContent = text;
    } catch (err) {
        result.textContent = '错误: ' + err;
    }
});