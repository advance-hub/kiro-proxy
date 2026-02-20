package kiro

import "net/http"

// HandleAdminPage GET /admin
func HandleAdminPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(adminHTML))
}

const adminHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>卡密管理后台</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:#f0f2f5;color:#1d2129;min-height:100vh}
.header{background:#fff;border-bottom:1px solid #e5e6eb;padding:16px 24px;display:flex;align-items:center;justify-content:space-between;position:sticky;top:0;z-index:100}
.header h1{font-size:20px;font-weight:600;color:#1d2129}
.header .actions{display:flex;gap:8px}
.container{max-width:1400px;margin:0 auto;padding:24px}
.stats{display:grid;grid-template-columns:repeat(3,1fr);gap:16px;margin-bottom:24px}
.stat-card{background:#fff;border-radius:8px;padding:20px;border:1px solid #e5e6eb}
.stat-card .label{font-size:14px;color:#86909c;margin-bottom:8px}
.stat-card .value{font-size:28px;font-weight:600}
.stat-card .value.total{color:#165dff}
.stat-card .value.activated{color:#00b42a}
.stat-card .value.unused{color:#ff7d00}
.toolbar{background:#fff;border-radius:8px 8px 0 0;padding:16px;border:1px solid #e5e6eb;border-bottom:none;display:flex;align-items:center;gap:12px;flex-wrap:wrap}
.toolbar .left{display:flex;gap:8px;align-items:center;flex:1;flex-wrap:wrap}
.toolbar .right{display:flex;gap:8px;align-items:center}
.search-input{padding:6px 12px;border:1px solid #c9cdd4;border-radius:4px;font-size:14px;width:240px;outline:none;transition:border-color .2s}
.search-input:focus{border-color:#165dff}
select{padding:6px 12px;border:1px solid #c9cdd4;border-radius:4px;font-size:14px;outline:none;background:#fff;cursor:pointer}
select:focus{border-color:#165dff}
.btn{padding:6px 16px;border-radius:4px;font-size:14px;cursor:pointer;border:1px solid transparent;transition:all .2s;display:inline-flex;align-items:center;gap:4px;white-space:nowrap}
.btn-primary{background:#165dff;color:#fff}.btn-primary:hover{background:#4080ff}
.btn-success{background:#00b42a;color:#fff}.btn-success:hover{background:#23c343}
.btn-warning{background:#ff7d00;color:#fff}.btn-warning:hover{background:#ff9a2e}
.btn-danger{background:#f53f3f;color:#fff}.btn-danger:hover{background:#f76560}
.btn-outline{background:#fff;color:#4e5969;border-color:#c9cdd4}.btn-outline:hover{border-color:#165dff;color:#165dff}
.btn-sm{padding:2px 8px;font-size:12px}
.table-wrap{background:#fff;border:1px solid #e5e6eb;border-radius:0 0 8px 8px;overflow-x:auto}
table{width:100%;border-collapse:collapse;font-size:14px}
thead th{background:#f7f8fa;padding:12px 16px;text-align:left;font-weight:500;color:#4e5969;border-bottom:1px solid #e5e6eb;white-space:nowrap;position:sticky;top:0}
tbody td{padding:10px 16px;border-bottom:1px solid #f2f3f5}
tbody tr:hover{background:#f7f8fa}
.code-text{font-family:'SF Mono',Monaco,'Courier New',monospace;font-size:13px;font-weight:500;letter-spacing:.5px}
.badge{display:inline-block;padding:2px 8px;border-radius:10px;font-size:12px;font-weight:500}
.badge-success{background:#e8ffea;color:#00b42a}
.badge-default{background:#f2f3f5;color:#86909c}
.machine-id{max-width:180px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:12px;color:#86909c;font-family:monospace}
.checkbox{width:16px;height:16px;cursor:pointer;accent-color:#165dff}
.modal-overlay{display:none;position:fixed;top:0;left:0;width:100%;height:100%;background:rgba(0,0,0,.45);z-index:200;justify-content:center;align-items:center}
.modal-overlay.show{display:flex}
.modal{background:#fff;border-radius:8px;padding:24px;width:480px;max-width:90vw;max-height:80vh;overflow-y:auto}
.modal h3{font-size:16px;font-weight:600;margin-bottom:16px}
.modal .form-group{margin-bottom:16px}
.modal label{display:block;font-size:14px;color:#4e5969;margin-bottom:6px}
.modal input,.modal textarea{width:100%;padding:8px 12px;border:1px solid #c9cdd4;border-radius:4px;font-size:14px;outline:none;transition:border-color .2s}
.modal input:focus,.modal textarea:focus{border-color:#165dff}
.modal textarea{resize:vertical;min-height:100px;font-family:monospace}
.modal .form-tip{font-size:12px;color:#86909c;margin-top:4px}
.modal .modal-footer{display:flex;justify-content:flex-end;gap:8px;margin-top:20px}
.toast{position:fixed;top:24px;right:24px;padding:12px 20px;border-radius:4px;font-size:14px;color:#fff;z-index:300;animation:fadeIn .3s}
.toast.success{background:#00b42a}.toast.error{background:#f53f3f}.toast.info{background:#165dff}
@keyframes fadeIn{from{opacity:0;transform:translateY(-10px)}to{opacity:1;transform:translateY(0)}}
.empty{text-align:center;padding:60px 20px;color:#86909c}
.pagination{display:flex;align-items:center;justify-content:space-between;padding:12px 16px;background:#fff;border-top:1px solid #e5e6eb}
.pagination .info{font-size:13px;color:#86909c}
.pagination .pages{display:flex;gap:4px;align-items:center}
.page-btn{padding:4px 10px;border:1px solid #c9cdd4;border-radius:4px;background:#fff;cursor:pointer;font-size:13px}
.page-btn:hover{border-color:#165dff;color:#165dff}
.page-btn.active{background:#165dff;color:#fff;border-color:#165dff}
.page-btn:disabled{opacity:.4;cursor:not-allowed}
.selected-bar{background:#e8f3ff;padding:8px 16px;display:flex;align-items:center;gap:12px;font-size:14px;color:#165dff}
.selected-bar .count{font-weight:600}
.section-title{font-size:18px;font-weight:600;margin:32px 0 16px;padding-bottom:12px;border-bottom:2px solid #e5e6eb;color:#1d2129}
</style>
</head>
<body>
<div class="header">
  <h1>管理后台</h1>
  <div class="actions"><button class="btn btn-outline" onclick="location.reload()">刷新</button></div>
</div>
<div class="container">
<h2 class="section-title">激活码管理</h2>
  <div class="stats">
    <div class="stat-card"><div class="label">总数</div><div class="value total" id="statTotal">-</div></div>
    <div class="stat-card"><div class="label">已激活</div><div class="value activated" id="statActivated">-</div></div>
    <div class="stat-card"><div class="label">未使用</div><div class="value unused" id="statUnused">-</div></div>
  </div>
  <div class="toolbar">
    <div class="left">
      <input type="text" class="search-input" id="searchInput" placeholder="搜索激活码 / 机器码..." oninput="renderTable()">
      <select id="filterStatus" onchange="renderTable()">
        <option value="all">全部状态</option>
        <option value="activated">已激活</option>
        <option value="unused">未使用</option>
      </select>
    </div>
    <div class="right">
      <button class="btn btn-primary" onclick="showAddModal()">+ 添加卡密</button>
      <div style="position:relative;display:inline-block">
        <button class="btn btn-success" onclick="toggleExportMenu()">导出</button>
        <div id="exportMenu" style="display:none;position:absolute;right:0;top:36px;background:#fff;border:1px solid #e5e6eb;border-radius:4px;box-shadow:0 4px 12px rgba(0,0,0,0.1);z-index:50;min-width:140px">
          <div style="padding:8px 16px;cursor:pointer;font-size:14px;white-space:nowrap" onmouseover="this.style.background='#f7f8fa'" onmouseout="this.style.background=''" onclick="exportCodes('all')">导出全部</div>
          <div style="padding:8px 16px;cursor:pointer;font-size:14px;white-space:nowrap" onmouseover="this.style.background='#f7f8fa'" onmouseout="this.style.background=''" onclick="exportCodes('activated')">导出已激活</div>
          <div style="padding:8px 16px;cursor:pointer;font-size:14px;white-space:nowrap" onmouseover="this.style.background='#f7f8fa'" onmouseout="this.style.background=''" onclick="exportCodes('unused')">导出未使用</div>
        </div>
      </div>
    </div>
  </div>
  <div id="selectedBar" class="selected-bar" style="display:none">
    <span>已选择 <span class="count" id="selectedCount">0</span> 项</span>
    <button class="btn btn-sm btn-primary" onclick="showBatchExpiryModal()">批量设置有效期</button>
    <button class="btn btn-sm btn-warning" onclick="showBatchTunnelModal()">设置穿透天数</button>
    <button class="btn btn-sm btn-outline" onclick="batchReset()">重置绑定</button>
    <button class="btn btn-sm btn-danger" onclick="batchDelete()">批量删除</button>
    <button class="btn btn-sm btn-outline" onclick="clearSelection()">取消选择</button>
  </div>
  <div class="table-wrap">
    <table>
      <thead><tr>
        <th><input type="checkbox" class="checkbox" id="selectAll" onchange="toggleSelectAll()"></th>
        <th>激活码</th><th>用户名</th><th>激活状态</th><th>过期日期</th><th>机器码</th><th>激活时间</th><th>穿透天数</th><th>操作</th>
      </tr></thead>
      <tbody id="tableBody"></tbody>
    </table>
    <div id="emptyState" class="empty" style="display:none">暂无数据</div>
  </div>
  <div class="pagination" id="pagination"></div>
</div>

<div class="modal-overlay" id="expiryModal"><div class="modal">
  <h3 id="expiryModalTitle">设置有效期</h3>
  <p id="expiryModalCurrent" style="font-size:13px;color:#86909c;margin-bottom:16px"></p>
  <div class="form-group"><label>有效期天数</label>
    <input type="number" id="expiryDaysInput" min="0" placeholder="输入天数（0=禁止使用，30=30天后过期）">
    <div class="form-tip">0 天 = 立即禁止使用 | 正数 = 指定天数后过期 | 负数 = 清空过期日期</div>
  </div>
  <div class="modal-footer">
    <button class="btn btn-outline" onclick="closeModal('expiryModal')">取消</button>
    <button class="btn btn-primary" onclick="submitExpiry()">确定</button>
  </div>
</div></div>

<div class="modal-overlay" id="addModal"><div class="modal">
  <h3>添加卡密</h3>
  <div class="form-group"><label>添加方式</label>
    <select id="addMode" onchange="toggleAddMode()" style="width:100%;padding:8px 12px;border:1px solid #c9cdd4;border-radius:4px;font-size:14px">
      <option value="random">随机生成</option><option value="custom">自定义输入</option>
    </select>
  </div>
  <div id="randomFields"><div class="form-group"><label>生成数量</label>
    <input type="number" id="addCount" value="10" min="1" max="1000">
    <div style="display:flex;gap:6px;margin-top:8px;flex-wrap:wrap">
      <button type="button" class="btn btn-sm btn-outline" onclick="document.getElementById('addCount').value=10">10</button>
      <button type="button" class="btn btn-sm btn-outline" onclick="document.getElementById('addCount').value=20">20</button>
      <button type="button" class="btn btn-sm btn-outline" onclick="document.getElementById('addCount').value=50">50</button>
      <button type="button" class="btn btn-sm btn-outline" onclick="document.getElementById('addCount').value=100">100</button>
      <button type="button" class="btn btn-sm btn-outline" onclick="document.getElementById('addCount').value=200">200</button>
      <button type="button" class="btn btn-sm btn-outline" onclick="document.getElementById('addCount').value=500">500</button>
      <button type="button" class="btn btn-sm btn-outline" onclick="document.getElementById('addCount').value=1000">1000</button>
    </div>
    <div class="form-tip">最多一次生成 1000 个</div>
  </div></div>
  <div id="customFields" style="display:none"><div class="form-group"><label>自定义卡密</label>
    <textarea id="customCodes" placeholder="一行一个卡密，格式：XXXX-XXXX-XXXX-XXXX"></textarea>
  </div></div>
  <div class="form-group"><label>穿透天数（tunnelDays）</label>
    <input type="number" id="addTunnelDays" value="0" min="0">
    <div class="form-tip">0 表示无穿透权限</div>
  </div>
  <div class="modal-footer">
    <button class="btn btn-outline" onclick="closeModal('addModal')">取消</button>
    <button class="btn btn-primary" onclick="submitAdd()">确认添加</button>
  </div>
</div></div>

<div class="modal-overlay" id="tunnelModal"><div class="modal">
  <h3>设置穿透天数</h3>
  <div class="form-group"><label>穿透天数</label>
    <input type="number" id="tunnelDaysInput" value="30" min="0">
    <div class="form-tip">将为选中的卡密设置穿透天数，0 表示无权限</div>
  </div>
  <div class="modal-footer">
    <button class="btn btn-outline" onclick="closeModal('tunnelModal')">取消</button>
    <button class="btn btn-primary" onclick="submitTunnelDays()">确认</button>
  </div>
</div></div>

<div class="modal-overlay" id="singleTunnelModal"><div class="modal">
  <h3>设置穿透天数</h3>
  <div class="form-group"><label>卡密: <span id="singleTunnelCode" class="code-text"></span></label></div>
  <div class="form-group"><label>穿透天数</label>
    <input type="number" id="singleTunnelDaysInput" value="30" min="0">
  </div>
  <div class="modal-footer">
    <button class="btn btn-outline" onclick="closeModal('singleTunnelModal')">取消</button>
    <button class="btn btn-primary" onclick="submitSingleTunnelDays()">确认</button>
  </div>
</div></div>

<script>
let allCodes=[];let allUsers={};let selected=new Set();let currentPage=1;const pageSize=20;

async function fetchCodes(){
  try{
    const [codesRes,usersRes]=await Promise.all([fetch('/api/admin/codes'),fetch('/api/admin/user-credentials')]);
    const codesData=await codesRes.json();
    const usersData=await usersRes.json();
    if(codesData.success){
      allCodes=codesData.codes||[];
      document.getElementById('statTotal').textContent=codesData.stats.total;
      document.getElementById('statActivated').textContent=codesData.stats.activated;
      document.getElementById('statUnused').textContent=codesData.stats.unused;
    }
    if(Array.isArray(usersData)){
      allUsers={};
      usersData.forEach(u=>{allUsers[u.activation_code]=u;});
    }
    renderTable();
  }catch(e){console.error('获取数据失败',e);}
}

function getFiltered(){
  const search=document.getElementById('searchInput').value.trim().toUpperCase();
  const status=document.getElementById('filterStatus').value;
  return allCodes.filter(c=>{
    if(status==='activated'&&!c.active)return false;
    if(status==='unused'&&c.active)return false;
    if(search&&!c.code.includes(search)&&!(c.machineId||'').toUpperCase().includes(search))return false;
    return true;
  });
}

function renderTable(){
  const filtered=getFiltered();
  const totalPages=Math.max(1,Math.ceil(filtered.length/pageSize));
  if(currentPage>totalPages)currentPage=totalPages;
  const start=(currentPage-1)*pageSize;
  const pageData=filtered.slice(start,start+pageSize);
  const tbody=document.getElementById('tableBody');
  if(pageData.length===0){tbody.innerHTML='';document.getElementById('emptyState').style.display='block';}
  else{
    document.getElementById('emptyState').style.display='none';
    tbody.innerHTML=pageData.map(c=>{
      const checked=selected.has(c.code)?'checked':'';
      const user=allUsers[c.code]||{};
      const userName=user.user_name||'<span style="color:#c9cdd4">-</span>';
      const statusBadge=c.active?'<span class="badge badge-success">已激活</span>':'<span class="badge badge-default">未使用</span>';
      const expired=c.expiresDate&&new Date(c.expiresDate)<new Date();
      const expiresDate=c.expiresDate?(expired?'<span style="color:#f53f3f">'+c.expiresDate+' (已过期)</span>':c.expiresDate):'<span style="color:#c9cdd4">未设置</span>';
      const mid=c.machineId?'<span class="machine-id" title="'+c.machineId+'">'+c.machineId+'</span>':'<span style="color:#c9cdd4">-</span>';
      const at=c.activatedAt?new Date(c.activatedAt).toLocaleString('zh-CN'):'<span style="color:#c9cdd4">-</span>';
      return '<tr><td><input type="checkbox" class="checkbox" '+checked+' onchange="toggleSelect(\''+c.code+'\')"></td>'+
        '<td><span class="code-text">'+c.code+'</span></td><td>'+userName+'</td><td>'+statusBadge+'</td><td>'+expiresDate+'</td><td>'+mid+'</td>'+
        '<td style="font-size:13px;color:#4e5969">'+at+'</td><td><span style="font-weight:500">'+(c.tunnelDays||0)+'</span> 天</td>'+
        '<td><button class="btn btn-sm btn-outline" onclick="showExpiryModal(\''+c.code+'\',\''+(c.expiresDate||'')+'\')">设有效期</button> '+
        '<button class="btn btn-sm btn-outline" onclick="showSingleTunnelModal(\''+c.code+'\','+(c.tunnelDays||0)+')">设天数</button> '+
        (c.active?'<button class="btn btn-sm btn-warning" onclick="resetSingle(\''+c.code+'\')">重置</button> ':'')+
        '<button class="btn btn-sm btn-danger" onclick="deleteSingle(\''+c.code+'\')">删除</button></td></tr>';
    }).join('');
  }
  updateSelectedBar();renderPagination(filtered.length,totalPages);
}

function renderPagination(total,totalPages){
  const pg=document.getElementById('pagination');
  let html='<div class="info">共 '+total+' 条</div><div class="pages">';
  html+='<button class="page-btn" onclick="goPage('+(currentPage-1)+')" '+(currentPage<=1?'disabled':'')+'>&lt;</button>';
  const maxShow=7;let startP=Math.max(1,currentPage-3);let endP=Math.min(totalPages,startP+maxShow-1);
  if(endP-startP<maxShow-1)startP=Math.max(1,endP-maxShow+1);
  for(let i=startP;i<=endP;i++)html+='<button class="page-btn '+(i===currentPage?'active':'')+'" onclick="goPage('+i+')">'+i+'</button>';
  html+='<button class="page-btn" onclick="goPage('+(currentPage+1)+')" '+(currentPage>=totalPages?'disabled':'')+'>&gt;</button></div>';
  pg.innerHTML=html;
}

function goPage(p){const f=getFiltered();const tp=Math.max(1,Math.ceil(f.length/pageSize));if(p>=1&&p<=tp){currentPage=p;renderTable();}}
function toggleSelect(code){if(selected.has(code))selected.delete(code);else selected.add(code);updateSelectedBar();}
function toggleSelectAll(){
  const all=document.getElementById('selectAll').checked;const f=getFiltered();
  const start=(currentPage-1)*pageSize;const pd=f.slice(start,start+pageSize);
  pd.forEach(c=>{if(all)selected.add(c.code);else selected.delete(c.code);});renderTable();
}
function clearSelection(){selected.clear();document.getElementById('selectAll').checked=false;renderTable();}
function updateSelectedBar(){
  const bar=document.getElementById('selectedBar');
  if(selected.size>0){bar.style.display='flex';document.getElementById('selectedCount').textContent=selected.size;}
  else{bar.style.display='none';}
}
function showToast(msg,type='success'){const t=document.createElement('div');t.className='toast '+type;t.textContent=msg;document.body.appendChild(t);setTimeout(()=>t.remove(),3000);}
function showAddModal(){document.getElementById('addModal').classList.add('show');}
function showBatchTunnelModal(){document.getElementById('tunnelModal').classList.add('show');}
function closeModal(id){document.getElementById(id).classList.remove('show');}
function toggleAddMode(){const m=document.getElementById('addMode').value;document.getElementById('randomFields').style.display=m==='random'?'block':'none';document.getElementById('customFields').style.display=m==='custom'?'block':'none';}
function showSingleTunnelModal(code,days){document.getElementById('singleTunnelCode').textContent=code;document.getElementById('singleTunnelDaysInput').value=days;document.getElementById('singleTunnelModal').dataset.code=code;document.getElementById('singleTunnelModal').classList.add('show');}

async function submitAdd(){
  const mode=document.getElementById('addMode').value;const tunnelDays=parseInt(document.getElementById('addTunnelDays').value)||0;
  let body={tunnelDays};
  if(mode==='random'){body.count=parseInt(document.getElementById('addCount').value)||10;}
  else{const text=document.getElementById('customCodes').value.trim();if(!text)return showToast('请输入卡密','error');body.customCodes=text.split('\n').map(s=>s.trim()).filter(Boolean);}
  try{const res=await fetch('/api/admin/codes',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});const data=await res.json();if(data.success){showToast(data.message);closeModal('addModal');fetchCodes();}else showToast(data.message,'error');}catch(e){showToast('添加失败','error');}
}
async function submitTunnelDays(){
  const days=parseInt(document.getElementById('tunnelDaysInput').value);if(isNaN(days))return showToast('请输入有效天数','error');
  try{const res=await fetch('/api/admin/codes/update',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({codesToUpdate:[...selected],tunnelDays:days})});const data=await res.json();if(data.success){showToast(data.message);closeModal('tunnelModal');selected.clear();fetchCodes();}else showToast(data.message,'error');}catch(e){showToast('更新失败','error');}
}
async function submitSingleTunnelDays(){
  const code=document.getElementById('singleTunnelModal').dataset.code;const days=parseInt(document.getElementById('singleTunnelDaysInput').value);if(isNaN(days))return showToast('请输入有效天数','error');
  try{const res=await fetch('/api/admin/codes/update',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({codesToUpdate:[code],tunnelDays:days})});const data=await res.json();if(data.success){showToast(data.message);closeModal('singleTunnelModal');fetchCodes();}else showToast(data.message,'error');}catch(e){showToast('更新失败','error');}
}
async function batchDelete(){
  if(!confirm('确定要删除选中的 '+selected.size+' 个卡密吗？'))return;
  try{const res=await fetch('/api/admin/codes/delete',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({codesToDelete:[...selected]})});const data=await res.json();if(data.success){showToast(data.message);selected.clear();fetchCodes();}else showToast(data.message,'error');}catch(e){showToast('删除失败','error');}
}
async function batchReset(){
  if(!confirm('确定要重置选中的 '+selected.size+' 个卡密的绑定吗？'))return;
  try{const res=await fetch('/api/admin/codes/reset',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({codesToReset:[...selected]})});const data=await res.json();if(data.success){showToast(data.message);selected.clear();fetchCodes();}else showToast(data.message,'error');}catch(e){showToast('重置失败','error');}
}
async function deleteSingle(code){
  if(!confirm('确定要删除卡密 '+code+' 吗？'))return;
  try{const res=await fetch('/api/admin/codes/delete',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({codesToDelete:[code]})});const data=await res.json();if(data.success){showToast(data.message);selected.delete(code);fetchCodes();}else showToast(data.message,'error');}catch(e){showToast('删除失败','error');}
}
async function resetSingle(code){
  if(!confirm('确定要重置卡密 '+code+' 的绑定吗？'))return;
  try{const res=await fetch('/api/admin/codes/reset',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({codesToReset:[code]})});const data=await res.json();if(data.success){showToast(data.message);fetchCodes();}else showToast(data.message,'error');}catch(e){showToast('重置失败','error');}
}
function toggleExportMenu(){const m=document.getElementById('exportMenu');m.style.display=m.style.display==='none'?'block':'none';}
document.addEventListener('click',e=>{if(!e.target.closest('#exportMenu')&&!e.target.closest('[onclick*="toggleExportMenu"]'))document.getElementById('exportMenu').style.display='none';});
function exportCodes(type){window.open('/api/admin/codes/export?type='+type,'_blank');document.getElementById('exportMenu').style.display='none';}
function showExpiryModal(code,currentExpiry){
  document.getElementById('expiryModal').dataset.code=code;
  document.getElementById('expiryModal').dataset.batch='false';
  document.getElementById('expiryModalTitle').textContent='设置有效期: '+code;
  document.getElementById('expiryModalCurrent').textContent='当前过期日期: '+(currentExpiry||'未设置');
  document.getElementById('expiryDaysInput').value='30';
  document.getElementById('expiryModal').classList.add('show');
}
function showBatchExpiryModal(){
  if(selected.size===0)return showToast('请先选择激活码','error');
  document.getElementById('expiryModal').dataset.batch='true';
  document.getElementById('expiryModalTitle').textContent='批量设置有效期 ('+selected.size+' 个激活码)';
  document.getElementById('expiryModalCurrent').textContent='将为选中的 '+selected.size+' 个激活码设置相同的有效期';
  document.getElementById('expiryDaysInput').value='30';
  document.getElementById('expiryModal').classList.add('show');
}
async function submitExpiry(){
  const isBatch=document.getElementById('expiryModal').dataset.batch==='true';
  const days=parseInt(document.getElementById('expiryDaysInput').value);
  if(isNaN(days))return showToast('请输入有效天数','error');
  if(isBatch){
    try{const res=await fetch('/api/admin/user-credentials/batch-set-expiry',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({activation_codes:[...selected],days})});
      const data=await res.json();if(data.success){showToast(data.message);closeModal('expiryModal');selected.clear();fetchCodes();}else showToast(data.message||'设置失败','error');
    }catch(e){showToast('设置失败','error');}
  }else{
    const code=document.getElementById('expiryModal').dataset.code;
    try{const res=await fetch('/api/admin/user-credentials/'+code+'/set-expiry',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({days})});
      const data=await res.json();if(data.success){showToast(data.message);closeModal('expiryModal');fetchCodes();}else showToast(data.message||'设置失败','error');
    }catch(e){showToast('设置失败','error');}
  }
}
document.querySelectorAll('.modal-overlay').forEach(el=>{el.addEventListener('click',e=>{if(e.target===el)el.classList.remove('show');});});
fetchCodes();
</script>
</body>
</html>`
