// Render the handbook using the project's existing Markdown dependencies.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { createRequire } from 'node:module';
const directory = path.dirname(fileURLToPath(import.meta.url));
const require = createRequire(path.join(directory, '../../../frontend/package.json'));
const React = require('react');
const {renderToStaticMarkup} = require('react-dom/server');
const {default: Markdown} = await import(require.resolve('react-markdown'));
const {default: gfm} = await import(require.resolve('remark-gfm'));
const source = fs.readFileSync(path.join(directory, '操作手册.md'), 'utf8');
const annotations = JSON.parse(fs.readFileSync(path.join(directory,'screenshots/annotations.json'),'utf8'));
const headings = [...source.matchAll(/^### (PM-\d+) · (.+)$/gm)].map(([,id,title])=>({id,title}));
if(headings.length !== 20 || new Set(headings.map(h=>h.id)).size !== 20) throw Error('Expected 20 unique cases');
const body = renderToStaticMarkup(React.createElement(Markdown, {
  remarkPlugins:[gfm],
  components:{
    h3:({children})=>React.createElement('h3',{id:String(children).match(/^PM-\d+/)?.[0]},children),
    img:({src,alt})=>{
      const note=annotations[path.basename(src??'')];
      if(!note)return React.createElement('img',{src,alt,style:{maxWidth:'100%'}});
      return React.createElement('span',{className:'annotated-shot'},
        React.createElement('span',{className:'shot-frame'},
          React.createElement('img',{src,alt}),
          ...note.rings.map(r=>React.createElement('span',{key:r.n,className:'shot-ring',style:{left:`${r.x/note.width*100}%`,top:`${r.y/note.height*100}%`,width:`${r.w/note.width*100}%`,height:`${r.h/note.height*100}%`},title:r.label},React.createElement('b',null,r.n)))),
        React.createElement('span',{className:'shot-caption'},note.caption+' '+note.rings.map(r=>`${r.n}：${r.label}`).join('；')));
    }
  },
  children:source,
}));
const escape = value=>String(value).replaceAll('&','&amp;').replaceAll('<','&lt;').replaceAll('"','&quot;');
const nav = headings.map(h=>`<a href="#${h.id}">${h.id} ${escape(h.title)}</a>`).join('');
const checklist = headings.map(h=>`<label>${h.id} ${escape(h.title)} <select aria-label="${h.id} 验收状态" data-case="${h.id}"><option>未执行</option><option>通过</option><option>失败</option><option>阻塞</option></select></label>`).join('');
fs.writeFileSync(path.join(directory,'index.html'),`<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Solo 本轮功能验收手册 · 初稿</title>
<style>*{box-sizing:border-box}html{scroll-behavior:smooth}body{margin:0;color:#213129;background:#f4f5f0;font:16px/1.85 system-ui,-apple-system,"PingFang SC",sans-serif}nav{position:fixed;inset:0 auto 0 0;width:270px;padding:26px 18px;overflow:auto;background:#173e31;color:white}nav strong{display:block;font-size:22px;margin-bottom:14px}nav a{display:block;color:#e3f0e6;text-decoration:none;font-size:13px;padding:6px 9px;border-radius:5px}nav a:hover,nav a:focus{background:#2c5946}main{margin:32px 36px 64px 302px;max-width:1020px;background:white;padding:36px 44px;border:1px solid #d4ddd3;border-radius:12px}h1{font-size:30px;line-height:1.4}h2{font-size:25px;margin-top:50px;border-bottom:2px solid #286246;padding-bottom:10px}h3{font-size:21px;margin-top:42px;padding:14px 18px;background:#edf4eb;scroll-margin-top:18px}a{color:#176449}table{border-collapse:collapse;width:100%;font-size:14px;display:block;overflow:auto}td,th{border:1px solid #d9e1d6;padding:9px 12px;vertical-align:top}th{background:#eff3ec;text-align:left}pre{background:#f2f4f1;border:1px solid #d4ddd3;border-radius:6px;padding:40px 18px 18px;white-space:pre-wrap;overflow-wrap:anywhere;position:relative;font-size:14px;line-height:1.65}code{font-family:ui-monospace,monospace}button,select{font:inherit;cursor:pointer;border:1px solid #648471;background:white;color:#214a34;border-radius:4px;padding:5px 10px}pre button{position:absolute;right:8px;top:6px;font-size:12px}li{margin-bottom:9px}.notice{background:#fff6d8;border-left:5px solid #d39b19;padding:16px 20px}.checklist{display:grid;gap:9px}.checklist label{display:flex;align-items:center;justify-content:space-between;border-bottom:1px solid #d4ddd3;padding:8px;gap:10px}footer{font-size:13px;color:#64736b}@media(max-width:960px){nav{position:static;width:auto;max-height:250px}main{margin:15px;padding:24px 18px}}@media print{nav,pre button,.toolbar{display:none}main{margin:0;padding:0;border:none}body{background:white;font-size:11pt}h2,h3{break-after:avoid}pre,tr{break-inside:avoid}a{color:inherit}table{display:table}}</style>
<style>.annotated-shot{display:block;margin:20px 0}.shot-frame{display:block;position:relative}.shot-frame img{display:block;width:100%;border:1px solid #ccd6cc}.shot-ring{position:absolute;border:3px solid #dd2238;border-radius:50%;pointer-events:none}.shot-ring b{position:absolute;left:-10px;top:-12px;border-radius:100%;background:#dd2238;color:white;font:700 14px/24px system-ui;width:24px;text-align:center;box-shadow:0 0 0 2px white}.shot-caption{display:block;font-size:13px;color:#536a5a;margin-top:8px}</style>
<nav aria-label="验收用例导航"><strong>Solo 验收手册</strong><a href="#start">准备与数据</a>${nav}<a href="#record">本次记录</a></nav><main id="start"><p class="notice">初稿：已通过 Browser Use 创建独立验收工作区并开始实拍。2026-09-10 已补 A02 创建表单与 Computer 离线实拍；当前账号的 Computer 离线，真实 Agent 试走仍阻塞，不能据此宣布全部通过。</p><div class="toolbar"><button onclick="window.print()">打印／存为 PDF</button></div>${body}<h2 id="record">本次验收记录</h2><p>只记录这一次实际操作。浏览器会尽量在本机保存选择，文件模式下可能无法持久保存；结束后可导出记录留存。</p><div class="checklist">${checklist}</div><p><button id="export">导出本次记录</button></p><footer>本页不加载外部资源，不向 Solo 或第三方发送验收记录。生成依据：同目录操作手册.md。截图原文件保持不变，编号与红圈是文档中的叠加标注。</footer></main>
<script>for(const pre of document.querySelectorAll('pre')){const button=document.createElement('button');button.textContent='复制内容';button.setAttribute('aria-label','复制下方样例');const value=pre.querySelector('code')?.textContent??pre.textContent;button.onclick=async()=>{try{await navigator.clipboard.writeText(value);button.textContent='已复制'}catch{const range=document.createRange();range.selectNodeContents(pre.querySelector('code')??pre);const selection=getSelection();selection.removeAllRanges();selection.addRange(range);button.textContent='已选中，请复制'}};pre.prepend(button)}const key='solo-pm-acceptance-20260909';let saved={};try{saved=JSON.parse(localStorage.getItem(key)||'{}')}catch{}for(const field of document.querySelectorAll('[data-case]')){if(['未执行','通过','失败','阻塞'].includes(saved[field.dataset.case]))field.value=saved[field.dataset.case];field.onchange=()=>{saved[field.dataset.case]=field.value;try{localStorage.setItem(key,JSON.stringify(saved))}catch{}}}document.querySelector('#export').onclick=()=>{const results=Object.fromEntries([...document.querySelectorAll('[data-case]')].map(field=>[field.dataset.case,field.value]));const url=URL.createObjectURL(new Blob([JSON.stringify({recorded_at:new Date().toISOString(),handbook_status:'draft-awaiting-real-screenshots-and-walkthrough',results},null,2)],{type:'application/json'}));const a=document.createElement('a');a.href=url;a.download='solo-pm-acceptance-record.json';a.click();URL.revokeObjectURL(url)};</script></html>`);
console.log('Generated index.html with 20 case links and 20 result controls.');
