package main

import (
	"bookget/config"
	"bookget/router"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// TaskStatus represents the status of a download task
type TaskStatus struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Status    string    `json:"status"` // pending, running, done, error
	Message   string    `json:"message"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Host      string    `json:"host"`
}

// TaskManager manages download tasks
type TaskManager struct {
	mu    sync.RWMutex
	tasks map[string]*TaskStatus
}

var taskMgr = &TaskManager{tasks: make(map[string]*TaskStatus)}

func (tm *TaskManager) Add(task *TaskStatus) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.tasks[task.ID] = task
}

func (tm *TaskManager) Get(id string) *TaskStatus {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.tasks[id]
}

func (tm *TaskManager) List() []*TaskStatus {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	list := make([]*TaskStatus, 0, len(tm.tasks))
	for _, t := range tm.tasks {
		list = append(list, t)
	}
	return list
}

// ========== API Handlers ==========

func handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, indexHTML)
}

func handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"version":    config.Version,
		"status":     "running",
		"task_count": len(taskMgr.tasks),
	})
}

func handleAPIDownload(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST only"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		URL    string `json:"url"`
		Sleep  int    `json:"sleep"`
		Thread int    `json:"thread"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		http.Error(w, `{"error":"invalid request, 'url' is required"}`, http.StatusBadRequest)
		return
	}

	rawURL := strings.TrimSpace(req.URL)
	if !strings.HasPrefix(rawURL, "http") {
		http.Error(w, `{"error":"URL must start with http"}`, http.StatusBadRequest)
		return
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		http.Error(w, `{"error":"invalid URL"}`, http.StatusBadRequest)
		return
	}

	taskID := fmt.Sprintf("%d", time.Now().UnixNano())
	task := &TaskStatus{
		ID:        taskID,
		URL:       rawURL,
		Status:    "pending",
		StartTime: time.Now(),
		Host:      parsedURL.Host,
	}
	taskMgr.Add(task)

	// Apply custom settings
	if req.Sleep > 0 {
		config.Conf.Sleep = req.Sleep
	}
	if req.Thread > 0 {
		config.Conf.Threads = req.Thread
	}

	// Run download in background
	go func() {
		task.Status = "running"
		log.Printf("[Task %s] Starting: %s", taskID, rawURL)

		result, err := router.FactoryRouter(parsedURL.Host, rawURL)

		task.EndTime = time.Now()
		if err != nil {
			task.Status = "error"
			task.Message = err.Error()
			log.Printf("[Task %s] Error: %s", taskID, err)
		} else if result != nil {
			task.Status = "done"
			if msg, ok := result["msg"]; ok && msg != nil {
				task.Message = fmt.Sprintf("%v", msg)
			} else {
				task.Message = "download completed"
			}
			log.Printf("[Task %s] Done", taskID)
		} else {
			task.Status = "done"
			task.Message = "download completed"
		}
	}()

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"task_id": taskID,
		"status":  "queued",
	})
}

func handleAPITasks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	tasks := taskMgr.List()
	json.NewEncoder(w).Encode(tasks)
}

func handleAPITaskDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	taskID := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	task := taskMgr.Get(taskID)
	if task == nil {
		http.Error(w, `{"error":"task not found"}`, http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(task)
}

func handleAPIFiles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	dir := config.Conf.Directory
	if dir == "" {
		dir = filepath.Join(".", "downloads")
	}

	relativePath := strings.TrimPrefix(r.URL.Path, "/api/files")
	fullPath := filepath.Join(dir, relativePath)

	info, err := os.Stat(fullPath)
	if err != nil {
		http.Error(w, `{"error":"path not found"}`, http.StatusNotFound)
		return
	}

	if !info.IsDir() {
		http.ServeFile(w, r, fullPath)
		return
	}

	// List directory
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		http.Error(w, `{"error":"cannot read directory"}`, http.StatusInternalServerError)
		return
	}

	type FileInfo struct {
		Name    string    `json:"name"`
		Path    string    `json:"path"`
		IsDir   bool      `json:"is_dir"`
		Size    int64     `json:"size"`
		ModTime time.Time `json:"mod_time"`
	}

	files := make([]FileInfo, 0, len(entries))
	for _, e := range entries {
		fi, _ := e.Info()
		files = append(files, FileInfo{
			Name:    e.Name(),
			Path:    filepath.Join("/api/files", relativePath, e.Name()),
			IsDir:   e.IsDir(),
			Size:    fi.Size(),
			ModTime: fi.ModTime(),
		})
	}

	json.NewEncoder(w).Encode(files)
}

func handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodDelete {
		http.Error(w, `{"error":"DELETE only"}`, http.StatusMethodNotAllowed)
		return
	}

	taskID := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	if taskID == "" {
		http.Error(w, `{"error":"task id required"}`, http.StatusBadRequest)
		return
	}

	task := taskMgr.Get(taskID)
	if task == nil {
		http.Error(w, `{"error":"task not found"}`, http.StatusNotFound)
		return
	}

	taskMgr.mu.Lock()
	delete(taskMgr.tasks, taskID)
	taskMgr.mu.Unlock()

	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

// ========== Static file serving for downloads ==========

func handleDownloads(w http.ResponseWriter, r *http.Request) {
	dir := config.Conf.Directory
	if dir == "" {
		dir = filepath.Join(".", "downloads")
	}
	http.StripPrefix("/downloads/", http.FileServer(http.Dir(dir))).ServeHTTP(w, r)
}

// ========== Log streaming ==========

func handleAPILogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			tasks := taskMgr.List()
			data, _ := json.Marshal(tasks)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// ========== Main ==========

func main() {
	// Initialize default config for API mode
	initAPIConfig()

	mux := http.NewServeMux()

	// Static pages
	mux.HandleFunc("/", handleIndex)

	// API endpoints
	mux.HandleFunc("/api/status", handleAPIStatus)
	mux.HandleFunc("/api/download", handleAPIDownload)
	mux.HandleFunc("/api/tasks", func(w http.ResponseWriter, r *http.Request) {
		// Route: /api/tasks vs /api/tasks/{id}
		trimmed := strings.TrimPrefix(r.URL.Path, "/api/tasks")
		if trimmed == "" || trimmed == "/" {
			if r.Method == http.MethodDelete {
				handleDeleteTask(w, r)
			} else {
				handleAPITasks(w, r)
			}
		} else {
			handleAPITaskDetail(w, r)
		}
	})
	mux.HandleFunc("/api/files", handleAPIFiles)
	mux.HandleFunc("/api/files/", handleAPIFiles)
	mux.HandleFunc("/api/logs", handleAPILogs)

	// Serve downloaded files
	mux.HandleFunc("/downloads/", handleDownloads)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8088"
	}

	log.Printf("bookget-web v%s starting on :%s", config.Version, port)
	log.Printf("Downloads directory: %s", config.Conf.Directory)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}

func initAPIConfig() {
	// Set defaults for API/web mode
	dir := os.Getenv("DOWNLOAD_DIR")
	if dir == "" {
		dir = filepath.Join(".", "downloads")
	}
	_ = os.MkdirAll(dir, os.ModePerm)

	sleep := 3
	thread := 1
	maxConcurrent := 16

	if v := os.Getenv("SLEEP"); v != "" {
		fmt.Sscanf(v, "%d", &sleep)
	}
	if v := os.Getenv("THREAD"); v != "" {
		fmt.Sscanf(v, "%d", &thread)
	}
	if v := os.Getenv("MAX_CONCURRENT"); v != "" {
		fmt.Sscanf(v, "%d", &maxConcurrent)
	}

	config.Conf = config.Input{
		Directory:     dir,
		Sleep:         sleep,
		Threads:       thread,
		MaxConcurrent: maxConcurrent,
		UserAgent:     config.DefaultUserAgent(),
		FileExt:       ".jpg",
		Quality:       80,
		Retries:       3,
		Timeout:       300,
		CookieFile:    filepath.Join(dir, "cookie.txt"),
		HeaderFile:    filepath.Join(dir, "header.txt"),
	}
}

const indexHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>bookget Web - 下载管理器</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:#0f172a;color:#e2e8f0;min-height:100vh}
.container{max-width:1200px;margin:0 auto;padding:20px}
header{display:flex;align-items:center;justify-content:space-between;padding:16px 24px;background:#1e293b;border-radius:12px;margin-bottom:24px}
header h1{font-size:20px;color:#38bdf8}
header h1 span{color:#94a3b8;font-weight:400;font-size:14px;margin-left:8px}
.status-badge{display:inline-flex;align-items:center;gap:6px;padding:4px 12px;border-radius:20px;font-size:13px;background:#064e3b;color:#34d399}
.status-badge::before{content:'';width:8px;height:8px;border-radius:50%;background:#34d399;animation:pulse 2s infinite}
@keyframes pulse{0%,100%{opacity:1}50%{opacity:.4}}
.grid{display:grid;grid-template-columns:380px 1fr;gap:20px}
@media(max-width:900px){.grid{grid-template-columns:1fr}}
.card{background:#1e293b;border-radius:12px;padding:20px;border:1px solid #334155}
.card h2{font-size:15px;color:#94a3b8;margin-bottom:16px;text-transform:uppercase;letter-spacing:.5px}
.input-group{margin-bottom:14px}
.input-group label{display:block;font-size:13px;color:#94a3b8;margin-bottom:6px}
.input-group input{width:100%;padding:10px 14px;background:#0f172a;border:1px solid #334155;border-radius:8px;color:#e2e8f0;font-size:14px;outline:none;transition:border .2s}
.input-group input:focus{border-color:#38bdf8}
.btn{display:inline-flex;align-items:center;justify-content:center;gap:8px;padding:10px 20px;border:none;border-radius:8px;font-size:14px;font-weight:600;cursor:pointer;transition:all .2s}
.btn-primary{background:#0ea5e9;color:#fff;width:100%}
.btn-primary:hover{background:#0284c7}
.btn-sm{padding:5px 10px;font-size:12px}
.btn-danger{background:#dc2626;color:#fff}
.btn-danger:hover{background:#b91c1c}
.params-row{display:grid;grid-template-columns:1fr 1fr;gap:10px}
.task-list{max-height:calc(100vh - 280px);overflow-y:auto}
.task-item{display:flex;align-items:center;gap:12px;padding:12px;border-radius:8px;background:#0f172a;margin-bottom:8px;cursor:pointer;transition:background .2s}
.task-item:hover{background:#1a2744}
.task-dot{width:10px;height:10px;border-radius:50%;flex-shrink:0}
.task-dot.running{background:#facc15;animation:pulse 1s infinite}
.task-dot.done{background:#34d399}
.task-dot.error{background:#f87171}
.task-dot.pending{background:#64748b}
.task-info{flex:1;min-width:0}
.task-info .url{font-size:13px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;color:#cbd5e1}
.task-info .meta{font-size:11px;color:#64748b;margin-top:2px}
.task-actions{display:flex;gap:4px}
.file-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(200px,1fr));gap:10px}
.file-item{padding:10px;background:#0f172a;border-radius:8px;display:flex;align-items:center;gap:8px;font-size:13px;transition:background .2s}
.file-item:hover{background:#1a2744}
.file-item a{color:#38bdf8;text-decoration:none;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;flex:1}
.file-item a:hover{text-decoration:underline}
.file-item .size{color:#64748b;font-size:11px;flex-shrink:0}
.tabs{display:flex;gap:0;margin-bottom:16px;border-bottom:1px solid #334155}
.tab{padding:8px 16px;font-size:14px;color:#64748b;cursor:pointer;border-bottom:2px solid transparent;transition:all .2s}
.tab.active{color:#38bdf8;border-bottom-color:#38bdf8}
.tab:hover{color:#cbd5e1}
.empty{text-align:center;padding:40px;color:#64748b;font-size:14px}
.toast{position:fixed;top:20px;right:20px;padding:12px 20px;border-radius:8px;font-size:14px;z-index:1000;animation:slideIn .3s}
.toast.success{background:#064e3b;color:#34d399;border:1px solid #34d399}
.toast.error{background:#7f1d1d;color:#f87171;border:1px solid #f87171}
@keyframes slideIn{from{transform:translateX(100%);opacity:0}to{transform:translateX(0);opacity:1}}
</style>
</head>
<body>
<div class="container">
  <header>
    <h1>bookget Web <span>v` + config.Version + `</span></h1>
    <div class="status-badge">运行中</div>
  </header>
 <div class="grid">
    <div>
      <div class="card">
        <h2>新建下载任务</h2>
        <div class="input-group">
          <label>图书 URL</label>
          <input type="text" id="urlInput" placeholder="https://read.nlc.cn/..." />
        </div>
        <div class="params-row">
          <div class="input-group">
            <label>间隔 (秒)</label>
            <input type="number" id="sleepInput" value="3" min="1" max="60" />
          </div>
          <div class="input-group">
            <label>线程数</label>
            <input type="number" id="threadInput" value="1" min="1" max="16" />
          </div>
        </div>
        <button class="btn btn-primary" onclick="submitTask()">开始下载</button>
      </div>
      <div class="card" style="margin-top:16px">
        <h2>下载任务</h2>
        <div class="task-list" id="taskList">
          <div class="empty">暂无任务</div>
        </div>
      </div>
    </div>
    <div>
      <div class="card">
        <div class="tabs">
          <div class="tab active" onclick="switchTab('files',this)">已下载文件</div>
          <div class="tab" onclick="switchTab('log',this)">任务日志</div>
        </div>
        <div id="filesPanel">
          <div class="file-grid" id="fileGrid">
            <div class="empty">加载中...</div>
          </div>
        </div>
        <div id="logPanel" style="display:none">
          <pre id="logContent" style="font-size:12px;color:#94a3b8;max-height:500px;overflow-y:auto;background:#0f172a;padding:12px;border-radius:8px"></pre>
        </div>
      </div>
    </div>
  </div>
</div>
<script>
const API = '';
let tasks = [];

function showToast(msg, type='success') {
  const t = document.createElement('div');
  t.className = 'toast ' + type;
  t.textContent = msg;
  document.body.appendChild(t);
  setTimeout(() => t.remove(), 3000);
}

async function submitTask() {
  const url = document.getElementById('urlInput').value.trim();
  if (!url) return showToast('请输入 URL', 'error');
  if (!url.startsWith('http')) return showToast('URL 格式无效', 'error');
  const sleep = parseInt(document.getElementById('sleepInput').value) || 3;
  const thread = parseInt(document.getElementById('threadInput').value) || 1;
  try {
    const res = await fetch(API + '/api/download', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({url, sleep, thread})
    });
    const data = await res.json();
    if (data.task_id) {
      showToast('任务已提交: ' + data.task_id);
      document.getElementById('urlInput').value = '';
      refreshTasks();
    } else {
      showToast(data.error || '提交失败', 'error');
    }
  } catch(e) {
    showToast('网络错误: ' + e.message, 'error');
  }
}

async function refreshTasks() {
  try {
    const res = await fetch(API + '/api/tasks');
    tasks = await res.json();
    renderTasks();
  } catch(e) {}
}

function renderTasks() {
  const list = document.getElementById('taskList');
  if (!tasks.length) { list.innerHTML = '<div class="empty">暂无任务</div>'; return; }
  tasks.sort((a,b) => new Date(b.start_time) - new Date(a.start_time));
  list.innerHTML = tasks.map(t => {
    const time = new Date(t.start_time).toLocaleTimeString('zh-CN');
    return '<div class="task-item" onclick="showDetail(\'' + t.id + '\')">' +
      '<div class="task-dot ' + t.status + '"></div>' +
      '<div class="task-info"><div class="url" title="' + t.url + '">' + t.host + '</div>' +
      '<div class="meta">' + time + ' | ' + statusText(t.status) + (t.message ? ' - ' + t.message : '') + '</div></div>' +
      '<div class="task-actions"><button class="btn btn-sm btn-danger" onclick="event.stopPropagation();deleteTask(\'' + t.id + '\')">删除</button></div></div>';
  }).join('');
}

function statusText(s) {
  return {pending:'排队中',running:'下载中',done:'完成',error:'失败'}[s] || s;
}

async function deleteTask(id) {
  if (!confirm('确认删除此任务记录?')) return;
  await fetch(API + '/api/tasks/' + id, {method:'DELETE'});
  refreshTasks();
}

async function showDetail(id) {
  const task = tasks.find(t => t.id === id);
  if (!task) return;
  const pre = document.getElementById('logContent');
  pre.textContent = JSON.stringify(task, null, 2);
  switchTab('log', document.querySelectorAll('.tab')[1]);
}

async function loadFiles(path) {
  try {
    const res = await fetch(API + '/api/files' + (path || '/'));
    const ct = res.headers.get('Content-Type');
    if (ct && ct.includes('application/json')) {
      const files = await res.json();
      renderFiles(files);
    }
  } catch(e) {}
}

function renderFiles(files) {
  const grid = document.getElementById('fileGrid');
  if (!files || !files.length) { grid.innerHTML = '<div class="empty">暂无文件</div>'; return; }
  grid.innerHTML = files.map(f => {
    if (f.is_dir) {
      return '<div class="file-item" onclick="loadFiles(\'' + f.path + '/\')" style="cursor:pointer">' +
        '<span>📁</span><span style="flex:1;white-space:nowrap;overflow:hidden;text-overflow:ellipsis">' + f.name + '</span></div>';
    }
    const size = f.size > 1024*1024 ? (f.size/1024/1024).toFixed(1)+'MB' : (f.size/1024).toFixed(1)+'KB';
    const dlPath = f.path.replace('/api/files', '/downloads');
    return '<div class="file-item"><span>📄</span><a href="' + dlPath + '" target="_blank">' + f.name + '</a><span class="size">' + size + '</span></div>';
  }).join('');
}

function switchTab(name, el) {
  document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
  el.classList.add('active');
  document.getElementById('filesPanel').style.display = name === 'files' ? '' : 'none';
  document.getElementById('logPanel').style.display = name === 'log' ? '' : 'none';
}

setInterval(() => { refreshTasks(); }, 5000);
refreshTasks();
loadFiles('/');
</script>
</body>
</html>`
