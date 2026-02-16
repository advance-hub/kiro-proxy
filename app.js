import express from 'express'
import expressWs from 'express-ws'
import cors from 'cors'
import vm from 'vm'
import { fileURLToPath } from 'url'
import { dirname, join } from 'path'
import { readFileSync, writeFileSync, existsSync, mkdirSync, unlinkSync } from 'fs'
import { spawn } from 'child_process'
import { randomBytes } from 'crypto'
import * as auth from './auth.js'
import * as fileManager from './fileManager.js'
import path from 'path'
import fs from 'fs'
const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)

const app = express()
const PORT = 7777

// 启用 WebSocket 支持
expressWs(app)

// 中间件
app.use(cors())
app.use(express.json({ limit: '10mb' }))

// 数据存储目录
const DATA_DIR = join(__dirname, 'data')
const USER_FILES_DIR = join(DATA_DIR, 'user_files')
const HISTORY_PATH = join(DATA_DIR, 'history.json')
const TEMP_DIR = join(__dirname, 'temp')

// 确保数据目录存在
if (!existsSync(DATA_DIR)) {
  mkdirSync(DATA_DIR, { recursive: true })
}

if (!existsSync(USER_FILES_DIR)) {
  mkdirSync(USER_FILES_DIR, { recursive: true })
}

// 确保临时目录存在
if (!existsSync(TEMP_DIR)) {
  mkdirSync(TEMP_DIR, { recursive: true })
}

// 认证中间件
function authMiddleware(req, res, next) {
  const token = req.headers.authorization?.replace('Bearer ', '')
  const user = auth.verifyToken(token)
  
  if (!user) {
    return res.status(401).json({ error: '未授权，请先登录' })
  }
  
  req.user = user
  next()
}

// 可选认证中间件（允许访客访问）
function optionalAuthMiddleware(req, res, next) {
  const token = req.headers.authorization?.replace('Bearer ', '')
  const user = auth.verifyToken(token)
  req.user = user || null
  next()
}

// 初始化默认数据（已废弃，保留用于兼容）
function getDefaultData() {
  return [
    {
      key: 'root-1',
      title: 'my-project',
      isLeaf: false,
      children: [
        {
          key: 'file-1',
          title: 'main.js',
          isLeaf: true,
          language: 'javascript',
          content: `// 欢迎使用 Code Runner
// 在这里编写你的代码

function hello() {
  console.log('Hello, World!');
}

hello();

// 示例：两数之和
function twoSum(nums, target) {
  const map = new Map();
  for (let i = 0; i < nums.length; i++) {
    const complement = target - nums[i];
    if (map.has(complement)) {
      return [map.get(complement), i];
    }
    map.set(nums[i], i);
  }
  return [];
}

console.log(twoSum([2, 7, 11, 15], 9)); // [0, 1]
`
        },
        {
          key: 'file-2',
          title: 'utils.js',
          isLeaf: true,
          language: 'javascript',
          content: `// 工具函数

/**
 * 防抖函数
 */
function debounce(fn, delay) {
  let timer = null;
  return function(...args) {
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => {
      fn.apply(this, args);
    }, delay);
  };
}

/**
 * 节流函数
 */
function throttle(fn, delay) {
  let lastTime = 0;
  return function(...args) {
    const now = Date.now();
    if (now - lastTime >= delay) {
      fn.apply(this, args);
      lastTime = now;
    }
  };
}

console.log('工具函数已加载');
`
        },
        {
          key: 'folder-1',
          title: 'algorithms',
          isLeaf: false,
          children: [
            {
              key: 'file-3',
              title: 'sort.js',
              isLeaf: true,
              language: 'javascript',
              content: `// 排序算法

/**
 * 快速排序
 */
function quickSort(arr) {
  if (arr.length <= 1) return arr;
  
  const pivot = arr[Math.floor(arr.length / 2)];
  const left = arr.filter(x => x < pivot);
  const middle = arr.filter(x => x === pivot);
  const right = arr.filter(x => x > pivot);
  
  return [...quickSort(left), ...middle, ...quickSort(right)];
}

/**
 * 归并排序
 */
function mergeSort(arr) {
  if (arr.length <= 1) return arr;
  
  const mid = Math.floor(arr.length / 2);
  const left = mergeSort(arr.slice(0, mid));
  const right = mergeSort(arr.slice(mid));
  
  return merge(left, right);
}

function merge(left, right) {
  const result = [];
  let i = 0, j = 0;
  
  while (i < left.length && j < right.length) {
    if (left[i] <= right[j]) {
      result.push(left[i++]);
    } else {
      result.push(right[j++]);
    }
  }
  
  return result.concat(left.slice(i)).concat(right.slice(j));
}

// 测试
const arr = [64, 34, 25, 12, 22, 11, 90];
console.log('原数组:', arr);
console.log('快速排序:', quickSort([...arr]));
console.log('归并排序:', mergeSort([...arr]));
`
            },
            {
              key: 'file-4',
              title: 'search.js',
              isLeaf: true,
              language: 'javascript',
              content: `// 搜索算法

/**
 * 二分查找
 */
function binarySearch(arr, target) {
  let left = 0;
  let right = arr.length - 1;
  
  while (left <= right) {
    const mid = Math.floor((left + right) / 2);
    
    if (arr[mid] === target) {
      return mid;
    } else if (arr[mid] < target) {
      left = mid + 1;
    } else {
      right = mid - 1;
    }
  }
  
  return -1;
}

/**
 * 二分查找（递归版本）
 */
function binarySearchRecursive(arr, target, left = 0, right = arr.length - 1) {
  if (left > right) return -1;
  
  const mid = Math.floor((left + right) / 2);
  
  if (arr[mid] === target) return mid;
  if (arr[mid] < target) {
    return binarySearchRecursive(arr, target, mid + 1, right);
  }
  return binarySearchRecursive(arr, target, left, mid - 1);
}

// 测试
const sortedArr = [1, 3, 5, 7, 9, 11, 13, 15];
console.log('数组:', sortedArr);
console.log('查找 7 的位置:', binarySearch(sortedArr, 7));
console.log('查找 6 的位置:', binarySearch(sortedArr, 6));
`
            }
          ]
        }
      ]
    }
  ]
}

// 读取文件数据
function readFilesData() {
  try {
    if (existsSync(FILES_PATH)) {
      const data = readFileSync(FILES_PATH, 'utf-8')
      return JSON.parse(data)
    }
  } catch (error) {
    console.error('读取文件数据失败:', error)
  }
  return getDefaultData()
}

// 保存文件数据
function saveFilesData(data) {
  try {
    writeFileSync(FILES_PATH, JSON.stringify(data, null, 2), 'utf-8')
    return true
  } catch (error) {
    console.error('保存文件数据失败:', error)
    return false
  }
}

// 访客模式的用户数据（不再使用，保留用于兼容）
function getUserData() {
  return {
    userId: 'guest',
    username: '访客',
    avatar: '',
    statistics: {
      totalProblems: 0,
      todayProblems: 0,
      streak: 0,
      longestStreak: 0
    },
    heatmap: {},
    lastActiveDate: null
  }
}

// 访客模式不保存数据
function saveUserData(data) {
  // 访客模式不保存统计数据
  return true
}

// 获取历史记录
function getHistory() {
  try {
    if (existsSync(HISTORY_PATH)) {
      const data = readFileSync(HISTORY_PATH, 'utf-8')
      return JSON.parse(data)
    }
  } catch (error) {
    console.error('读取历史记录失败:', error)
  }
  return []
}

// 保存历史记录
function saveHistory(data) {
  try {
    writeFileSync(HISTORY_PATH, JSON.stringify(data, null, 2), 'utf-8')
    return true
  } catch (error) {
    console.error('保存历史记录失败:', error)
    return false
  }
}

// 添加历史记录
function addHistoryRecord(record) {
  const history = getHistory()
  history.unshift({
    id: `record_${Date.now()}`,
    fileName: record.fileName,
    code: record.code,           // 代码内容
    exitCode: record.exitCode,   // 退出码
    language: record.language,
    executionTime: record.executionTime,
    timestamp: new Date().toISOString()
  })
  
  // 只保留最近 100 条记录
  if (history.length > 100) {
    history.splice(100)
  }
  
  saveHistory(history)
  return true
}

// 旧的updateUserStats函数已移除，现在使用auth.updateUserStats(userId)

// ==================== 认证 API ====================

// 注册
app.post('/api/auth/register', (req, res) => {
  try {
    const { username, password } = req.body
    
    if (!username || !password) {
      return res.status(400).json({ error: '用户名和密码不能为空' })
    }
    
    if (username.length < 3 || username.length > 20) {
      return res.status(400).json({ error: '用户名长度必须在3-20个字符之间' })
    }
    
    if (password.length < 6) {
      return res.status(400).json({ error: '密码长度至少6个字符' })
    }
    
    const result = auth.register(username, password)
    
    if (result.success) {
      res.json(result)
    } else {
      res.status(400).json(result)
    }
  } catch (error) {
    res.status(500).json({ error: '注册失败' })
  }
})

// 登录
app.post('/api/auth/login', (req, res) => {
  try {
    const { username, password } = req.body
    
    if (!username || !password) {
      return res.status(400).json({ error: '用户名和密码不能为空' })
    }
    
    const result = auth.login(username, password)
    
    if (result.success) {
      res.json(result)
    } else {
      res.status(401).json(result)
    }
  } catch (error) {
    res.status(500).json({ error: '登录失败' })
  }
})

// 登出
app.post('/api/auth/logout', authMiddleware, (req, res) => {
  try {
    const token = req.headers.authorization?.replace('Bearer ', '')
    auth.logout(token)
    res.json({ success: true, message: '登出成功' })
  } catch (error) {
    res.status(500).json({ error: '登出失败' })
  }
})

// 验证token
app.get('/api/auth/verify', (req, res) => {
  try {
    const token = req.headers.authorization?.replace('Bearer ', '')
    const user = auth.verifyToken(token)
    
    if (user) {
      res.json({ success: true, user })
    } else {
      res.status(401).json({ success: false, error: 'Token无效或已过期' })
    }
  } catch (error) {
    res.status(500).json({ error: '验证失败' })
  }
})

// ==================== 文件管理 API ====================

// 获取用户文件树
app.get('/api/files', authMiddleware, (req, res) => {
  try {
    const files = fileManager.getUserFiles(req.user.userId)
    res.json(files)
  } catch (error) {
    res.status(500).json({ error: '获取文件失败' })
  }
})

// 保存用户文件树
app.post('/api/files', authMiddleware, (req, res) => {
  try {
    const { treeData } = req.body
    if (!treeData) {
      return res.status(400).json({ error: '缺少文件数据' })
    }
    
    const success = fileManager.saveUserFiles(req.user.userId, treeData)
    if (success) {
      res.json({ message: '保存成功' })
    } else {
      res.status(500).json({ error: '保存失败' })
    }
  } catch (error) {
    res.status(500).json({ error: '保存文件失败' })
  }
})

// 获取模板库
app.get('/api/templates', (req, res) => {
  try {
    const templates = fileManager.getTemplates()
    res.json(templates)
  } catch (error) {
    res.status(500).json({ error: '获取模板失败' })
  }
})

// 导入模板到用户空间
app.post('/api/files/import-templates', authMiddleware, (req, res) => {
  try {
    const files = fileManager.importTemplates(req.user.userId)
    res.json({ success: true, files })
  } catch (error) {
    res.status(500).json({ error: '导入模板失败' })
  }
})

// 重置为模板
app.post('/api/files/reset-templates', authMiddleware, (req, res) => {
  try {
    const files = fileManager.resetToTemplates(req.user.userId)
    res.json({ success: true, files })
  } catch (error) {
    res.status(500).json({ error: '重置失败' })
  }
})

// ==================== 文件分享 API ====================

// 通过文件key获取公开文件
app.get('/api/file/:fileKey', (req, res) => {
  try {
    const { fileKey } = req.params
    const file = fileManager.getFileByKey(fileKey)
    
    if (!file) {
      return res.status(404).json({ error: '文件不存在或未公开' })
    }
    
    res.json(file)
  } catch (error) {
    res.status(500).json({ error: '获取文件失败' })
  }
})

// 获取root模板库
app.get('/api/root-templates', (req, res) => {
  try {
    const templates = fileManager.getRootTemplates()
    res.json(templates)
  } catch (error) {
    res.status(500).json({ error: '获取模板库失败' })
  }
})

// 获取社区公开文件列表（排除当前用户）
app.get('/api/community-files', (req, res) => {
  try {
    const token = req.headers.authorization?.replace('Bearer ', '')
    const user = auth.verifyToken(token)
    const currentUserId = user?.userId || null
    
    const limit = parseInt(req.query.limit) || 50
    const files = fileManager.getPublicFiles(limit, currentUserId)
    res.json(files)
  } catch (error) {
    res.status(500).json({ error: '获取社区文件列表失败' })
  }
})

// 获取所有公开文件列表（兼容旧接口）
app.get('/api/public-files', (req, res) => {
  try {
    const limit = parseInt(req.query.limit) || 50
    const files = fileManager.getPublicFiles(limit)
    res.json(files)
  } catch (error) {
    res.status(500).json({ error: '获取公开文件列表失败' })
  }
})

// 获取用户的公开文件列表
app.get('/api/user/public-files', authMiddleware, (req, res) => {
  try {
    const files = fileManager.getUserPublicFiles(req.user.userId)
    res.json(files)
  } catch (error) {
    res.status(500).json({ error: '获取公开文件列表失败' })
  }
})

// 获取文件详细信息（包含作者信息）
app.get('/api/file/:fileKey/info', (req, res) => {
  try {
    const { fileKey } = req.params
    const file = fileManager.getFileByKey(fileKey)
    
    if (!file) {
      return res.status(404).json({ error: '文件不存在或未公开' })
    }
    
    // 获取作者用户名
    const owner = auth.getUserById(file.ownerId)
    
    res.json({
      ...file,
      ownerUsername: owner?.username || '未知用户'
    })
  } catch (error) {
    res.status(500).json({ error: '获取文件信息失败' })
  }
})

// 更新文件公开状态
app.patch('/api/file/:fileKey/visibility', authMiddleware, (req, res) => {
  try {
    const { fileKey } = req.params
    const { isPublic } = req.body
    
    if (typeof isPublic !== 'boolean') {
      return res.status(400).json({ error: '参数错误' })
    }
    
    const success = fileManager.updateFileVisibility(req.user.userId, fileKey, isPublic)
    
    if (success) {
      res.json({ 
        success: true, 
        message: '更新成功',
        shareUrl: isPublic ? `/question/${fileKey}` : null
      })
    } else {
      res.status(403).json({ error: '文件不存在或无权修改' })
    }
  } catch (error) {
    res.status(500).json({ error: '更新文件状态失败' })
  }
})

// 复制分享文件到用户空间
app.post('/api/file/:fileKey/copy', authMiddleware, (req, res) => {
  try {
    const { fileKey } = req.params
    const { targetFolderId } = req.body
    const result = fileManager.copySharedFileToUser(req.user.userId, fileKey, targetFolderId)
    
    if (result.success) {
      res.json(result)
    } else {
      res.status(404).json(result)
    }
  } catch (error) {
    res.status(500).json({ error: '复制文件失败' })
  }
})

// ==================== 代码执行 API ====================

// 运行代码（使用 Node.js vm 模块，支持异步）
app.post('/api/run', async (req, res) => {
  try {
    const { code, language } = req.body
    
    if (language !== 'javascript') {
      return res.status(400).json({ error: '暂不支持该语言' })
    }

    const logs = []
    const asyncLogs = []
    
    // 创建自定义 console 对象
    const customConsole = {
      log: (...args) => {
        const content = args.map(arg => 
          typeof arg === 'object' ? JSON.stringify(arg, null, 2) : String(arg)
        ).join(' ')
        logs.push({ type: 'log', content })
      },
      info: (...args) => {
        const content = args.map(arg => 
          typeof arg === 'object' ? JSON.stringify(arg, null, 2) : String(arg)
        ).join(' ')
        logs.push({ type: 'info', content })
      },
      warn: (...args) => {
        const content = args.map(arg => 
          typeof arg === 'object' ? JSON.stringify(arg, null, 2) : String(arg)
        ).join(' ')
        logs.push({ type: 'warn', content })
      },
      error: (...args) => {
        const content = args.map(arg => 
          typeof arg === 'object' ? JSON.stringify(arg, null, 2) : String(arg)
        ).join(' ')
        logs.push({ type: 'error', content })
      }
    }

    // 异步日志收集器
    const asyncConsole = {
      log: (...args) => {
        const content = args.map(arg => 
          typeof arg === 'object' ? JSON.stringify(arg, null, 2) : String(arg)
        ).join(' ')
        asyncLogs.push({ type: 'log', content })
      },
      info: (...args) => {
        const content = args.map(arg => 
          typeof arg === 'object' ? JSON.stringify(arg, null, 2) : String(arg)
        ).join(' ')
        asyncLogs.push({ type: 'info', content })
      },
      warn: (...args) => {
        const content = args.map(arg => 
          typeof arg === 'object' ? JSON.stringify(arg, null, 2) : String(arg)
        ).join(' ')
        asyncLogs.push({ type: 'warn', content })
      },
      error: (...args) => {
        const content = args.map(arg => 
          typeof arg === 'object' ? JSON.stringify(arg, null, 2) : String(arg)
        ).join(' ')
        asyncLogs.push({ type: 'error', content })
      }
    }

    // 创建沙箱环境
    const sandbox = {
      console: customConsole,
      setTimeout: (fn, delay) => {
        return setTimeout(() => {
          try {
            // 临时替换 console 为异步版本
            const originalConsole = sandbox.console
            sandbox.console = asyncConsole
            fn()
            sandbox.console = originalConsole
          } catch (error) {
            asyncLogs.push({ type: 'error', content: error.message })
          }
        }, delay)
      },
      setInterval: (fn, delay) => {
        return setInterval(() => {
          try {
            const originalConsole = sandbox.console
            sandbox.console = asyncConsole
            fn()
            sandbox.console = originalConsole
          } catch (error) {
            asyncLogs.push({ type: 'error', content: error.message })
          }
        }, delay)
      },
      clearTimeout,
      clearInterval,
      Promise: class extends Promise {
        constructor(executor) {
          super((resolve, reject) => {
            executor(
              (value) => {
                resolve(value)
              },
              (reason) => {
                asyncLogs.push({ type: 'error', content: `Promise rejected: ${reason}` })
                reject(reason)
              }
            )
          })
        }
        
        then(onFulfilled, onRejected) {
          return super.then(
            onFulfilled ? (value) => {
              try {
                const originalConsole = sandbox.console
                sandbox.console = asyncConsole
                const result = onFulfilled(value)
                sandbox.console = originalConsole
                return result
              } catch (error) {
                asyncLogs.push({ type: 'error', content: error.message })
                throw error
              }
            } : undefined,
            onRejected ? (reason) => {
              try {
                const originalConsole = sandbox.console
                sandbox.console = asyncConsole
                const result = onRejected(reason)
                sandbox.console = originalConsole
                return result
              } catch (error) {
                asyncLogs.push({ type: 'error', content: error.message })
                throw error
              }
            } : undefined
          )
        }
      },
      // 添加常用的全局对象
      JSON,
      Math,
      Date,
      Array,
      Object,
      String,
      Number,
      Boolean,
      RegExp,
      Error,
      TypeError,
      ReferenceError,
      SyntaxError
    }

    try {
      // 包装代码以支持异步
      const wrappedCode = `
        (async () => {
          try {
            ${code}
          } catch (error) {
            console.error('运行时错误: ' + error.message)
          }
        })()
      `

      // 创建 VM 上下文
      const context = vm.createContext(sandbox)
      
      // 执行代码
      const script = new vm.Script(wrappedCode)
      const result = script.runInContext(context, {
        timeout: 5000,
        displayErrors: true
      })

      // 如果返回的是 Promise，等待它完成
      if (result && typeof result.then === 'function') {
        await result.catch(error => {
          asyncLogs.push({ type: 'error', content: error.message })
        })
      }

      // 等待异步操作完成
      await new Promise(resolve => setTimeout(resolve, 200))

      // 合并同步和异步日志
      const allLogs = [...logs, ...asyncLogs]
      res.json({ success: true, logs: allLogs })

    } catch (error) {
      logs.push({ type: 'error', content: error.message })
      res.json({ success: false, error: error.message, logs })
    }

  } catch (error) {
    res.status(500).json({ error: '运行代码失败' })
  }
})

// 获取用户数据
app.get('/api/user', optionalAuthMiddleware, (req, res) => {
  try {
    if (req.user) {
      const userData = auth.getUserById(req.user.userId)
      res.json(userData)
    } else {
      const userData = getUserData()
      res.json(userData)
    }
  } catch (error) {
    res.status(500).json({ error: '获取用户数据失败' })
  }
})

// 获取历史记录
app.get('/api/history', optionalAuthMiddleware, (req, res) => {
  try {
    let history
    if (req.user) {
      history = fileManager.getUserHistory(req.user.userId)
    } else {
      history = getHistory()
    }
    res.json(history)
  } catch (error) {
    res.status(500).json({ error: '获取历史记录失败' })
  }
})

// 删除历史记录
app.delete('/api/history/:id', optionalAuthMiddleware, (req, res) => {
  try {
    const { id } = req.params
    
    if (req.user) {
      const history = fileManager.getUserHistory(req.user.userId)
      const newHistory = history.filter(item => item.id !== id)
      fileManager.saveUserHistory(req.user.userId, newHistory)
    } else {
      const history = getHistory()
      const newHistory = history.filter(item => item.id !== id)
      saveHistory(newHistory)
    }
    
    res.json({ message: '删除成功' })
  } catch (error) {
    res.status(500).json({ error: '删除历史记录失败' })
  }
})

// 清空历史记录
app.delete('/api/history', optionalAuthMiddleware, (req, res) => {
  try {
    if (req.user) {
      fileManager.saveUserHistory(req.user.userId, [])
    } else {
      saveHistory([])
    }
    res.json({ message: '清空成功' })
  } catch (error) {
    res.status(500).json({ error: '清空历史记录失败' })
  }
})

// 健康检查
app.get('/api/health', (req, res) => {
  res.json({ status: 'ok', timestamp: new Date().toISOString() })
})

// WebSocket 路由
app.ws('/ws', (ws, req) => {
  console.log('🔌 WebSocket client connected')
  
  let userId = null

  ws.on('message', async (message) => {
    try {
      const data = JSON.parse(message.toString())
      
      if (data.type === 'auth') {
        const user = auth.verifyToken(data.token)
        if (user) {
          userId = user.userId
          console.log('✅ WebSocket认证成功, userId:', userId)
          ws.send(JSON.stringify({ type: 'auth', success: true }))
        } else {
          console.log('❌ WebSocket认证失败')
          ws.send(JSON.stringify({ type: 'auth', success: false }))
        }
      } else if (data.type === 'run') {
        console.log('🚀 执行代码, userId:', userId || '访客模式')
        await handleCodeExecution(ws, data.code, data.language, data.fileName, userId)
      }
    } catch (error) {
      ws.send(JSON.stringify({
        type: 'error',
        content: error.message
      }))
    }
  })

  ws.on('close', () => {
    console.log('🔌 WebSocket client disconnected')
  })

  ws.on('error', (error) => {
    console.error('WebSocket error:', error)
  })
})

// WebSocket 代码执行处理
async function handleCodeExecution(ws, codeContent, language, fileName = 'untitled.js', userId = null) {
  if (language !== 'javascript') {
    ws.send(JSON.stringify({
      type: 'error',
      content: '暂不支持该语言'
    }))
    ws.send(JSON.stringify({ type: 'complete' }))
    return
  }

  const startTime = Date.now()
  
  // 生成临时文件名
  const tempFileName = `temp_${randomBytes(8).toString('hex')}.js`
  const tempFilePath = join(TEMP_DIR, tempFileName)
  
  try {
    ws.send(JSON.stringify({ type: 'info', content: `▶ 开始执行 ${fileName}...` }))
    
    // 将代码写入临时文件
    writeFileSync(tempFilePath, codeContent, 'utf-8')
    
    // 使用 spawn 执行 node 命令
    const nodeProcess = spawn('node', [tempFilePath], {
      cwd: TEMP_DIR,
      env: process.env
    })
    
    // 捕获标准输出
    nodeProcess.stdout.on('data', (data) => {
      const output = data.toString()
      // 按行分割并发送
      output.split('\n').forEach(line => {
        if (line.trim()) {
          ws.send(JSON.stringify({ type: 'log', content: line }))
        }
      })
    })
    
    // 捕获标准错误
    nodeProcess.stderr.on('data', (data) => {
      const error = data.toString()
      error.split('\n').forEach(line => {
        if (line.trim()) {
          ws.send(JSON.stringify({ type: 'error', content: line }))
        }
      })
    })
    
    // 进程退出
    nodeProcess.on('close', (code) => {
      const executionTime = Date.now() - startTime
      
      // 删除临时文件
      try {
        unlinkSync(tempFilePath)
      } catch (err) {
        console.error('删除临时文件失败:', err)
      }
      
      // 保存历史记录并更新统计
      if (userId) {
        // 登录用户：保存到用户文件夹下的history.json
        const history = fileManager.getUserHistory(userId) || []
        history.unshift({
          id: `record_${Date.now()}`,
          fileName,
          code: codeContent,  // 保存代码内容
          exitCode: code,     // 保存退出码
          language,
          executionTime,
          timestamp: new Date().toISOString()
        })
        if (history.length > 100) {
          history.splice(100)
        }
        fileManager.saveUserHistory(userId, history)
        auth.updateUserStats(userId)
      } else {
        // 访客模式：保存到全局history.json
        addHistoryRecord({
          fileName,
          code: codeContent,
          exitCode: code,
          language,
          executionTime
        })
      }
      
      // 发送完成消息
      if (code === 0) {
        ws.send(JSON.stringify({ 
          type: 'complete', 
          executionTime 
        }))
      } else {
        ws.send(JSON.stringify({ 
          type: 'complete', 
          executionTime,
          warning: `进程退出码: ${code}`
        }))
      }
    })
    
    // 进程错误
    nodeProcess.on('error', (error) => {
      ws.send(JSON.stringify({ 
        type: 'error', 
        content: `执行失败: ${error.message}` 
      }))
      
      // 删除临时文件
      try {
        unlinkSync(tempFilePath)
      } catch (err) {
        console.error('删除临时文件失败:', err)
      }
      
      const executionTime = Date.now() - startTime
      ws.send(JSON.stringify({ type: 'complete', executionTime }))
    })
    
    // 设置超时（30秒）
    setTimeout(() => {
      if (!nodeProcess.killed) {
        nodeProcess.kill()
        ws.send(JSON.stringify({ 
          type: 'error', 
          content: '执行超时（30秒），已终止' 
        }))
      }
    }, 30000)
    
  } catch (error) {
    ws.send(JSON.stringify({ 
      type: 'error', 
      content: `执行失败: ${error.message}` 
    }))
    
    // 删除临时文件
    try {
      if (existsSync(tempFilePath)) {
        unlinkSync(tempFilePath)
      }
    } catch (err) {
      console.error('删除临时文件失败:', err)
    }
    
    const executionTime = Date.now() - startTime
    ws.send(JSON.stringify({ type: 'complete', executionTime }))
  }
}

// 全局错误处理，防止进程崩溃
process.on('uncaughtException', (error) => {
  console.error('❌ Uncaught Exception:', error)
  console.error(error.stack)
})

process.on('unhandledRejection', (reason, promise) => {
  console.error('❌ Unhandled Rejection at:', promise)
  console.error('Reason:', reason)
})

// 静态文件服务（生产环境）
const distPath = join(__dirname, '../dist')
if (existsSync(distPath)) {
  app.use(express.static(distPath))
  
  // SPA路由回退：所有非API路由都返回index.html
  app.get('*', (req, res) => {
    // 排除API路由和WebSocket路由
    if (!req.path.startsWith('/api') && !req.path.startsWith('/ws')) {
      res.sendFile(join(distPath, 'index.html'))
    }
  })
}


const CODES_FILE = path.join(__dirname, "codes.json");

function readCodes() {
  return JSON.parse(fs.readFileSync(CODES_FILE, "utf-8"));
}

function writeCodes(codes) {
  fs.writeFileSync(CODES_FILE, JSON.stringify(codes, null, 2), "utf-8");
}

app.post("/api/activate", (req, res) => {
  const { code, machineId } = req.body;
  if (!code || typeof code !== "string") {
    return res.status(400).json({ success: false, message: "缺少激活码" });
  }
  if (!machineId || typeof machineId !== "string") {
    return res.status(400).json({ success: false, message: "缺少机器码" });
  }

  const codes = readCodes();
  const trimmed = code.trim().toUpperCase();
  const entry = codes.find((c) => c.code === trimmed);

  if (!entry) {
    return res.json({ success: false, message: "激活码无效" });
  }

  if (entry.active && entry.machineId && entry.machineId !== machineId) {
    return res.json({ success: false, message: "该激活码已被其他设备使用" });
  }

  if (entry.active && entry.machineId === machineId) {
    return res.json({ success: true, message: "已激活" });
  }

  entry.active = true;
  entry.machineId = machineId;
  entry.activatedAt = new Date().toISOString();
  writeCodes(codes);

  return res.json({ success: true, message: "激活成功" });
});

app.get("/api/codeslist", (req, res) => {
  const codes = readCodes();
  const rows = codes.map(c =>
    `<tr><td>${c.code}</td><td>${c.active ? "✅" : "❌"}</td><td>${c.machineId || "-"}</td><td>${c.activatedAt || "-"}</td><td>${c.tunnelDays || 0}</td></tr>`
  ).join("");
  res.send(`<html><head><meta charset="UTF-8"><style>
    body{font-family:monospace;margin:40px}
    table{border-collapse:collapse}
    th,td{border:1px solid #ccc;padding:6px 12px}
    th{background:#eee}
  </style></head><body>
  <h3>激活码列表</h3>
  <table><tr><th>激活码</th><th>状态</th><th>机器码</th><th>激活时间</th><th>穿透天数</th></tr>${rows}</table>
  </body></html>`);
});

// ==================== 卡密管理 API ====================

// 生成随机激活码
function generateCode() {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789';
  const seg = () => Array.from({ length: 4 }, () => chars[Math.floor(Math.random() * chars.length)]).join('');
  return `${seg()}-${seg()}-${seg()}-${seg()}`;
}

// 获取所有卡密列表
app.get("/api/admin/codes", (req, res) => {
  try {
    const codes = readCodes();
    const stats = {
      total: codes.length,
      activated: codes.filter(c => c.active).length,
      unused: codes.filter(c => !c.active).length
    };
    res.json({ success: true, codes, stats });
  } catch (error) {
    res.status(500).json({ success: false, message: "读取卡密失败" });
  }
});

// 批量添加卡密
app.post("/api/admin/codes", (req, res) => {
  try {
    const { count, customCodes, tunnelDays } = req.body;
    const codes = readCodes();
    const added = [];
    const days = tunnelDays || 0;

    if (customCodes && Array.isArray(customCodes)) {
      for (const c of customCodes) {
        const trimmed = c.trim().toUpperCase();
        if (trimmed && !codes.find(x => x.code === trimmed)) {
          const entry = { code: trimmed, active: false, machineId: null, activatedAt: null, tunnelDays: days };
          codes.push(entry);
          added.push(trimmed);
        }
      }
    } else if (count && count > 0) {
      const num = Math.min(count, 1000);
      for (let i = 0; i < num; i++) {
        let newCode;
        do { newCode = generateCode(); } while (codes.find(x => x.code === newCode));
        const entry = { code: newCode, active: false, machineId: null, activatedAt: null, tunnelDays: days };
        codes.push(entry);
        added.push(newCode);
      }
    } else {
      return res.status(400).json({ success: false, message: "请提供 count 或 customCodes" });
    }

    writeCodes(codes);
    res.json({ success: true, message: `成功添加 ${added.length} 个卡密`, added });
  } catch (error) {
    res.status(500).json({ success: false, message: "添加卡密失败" });
  }
});

// 删除卡密（支持批量）
app.post("/api/admin/codes/delete", (req, res) => {
  try {
    const { codesToDelete } = req.body;
    if (!codesToDelete || !Array.isArray(codesToDelete)) {
      return res.status(400).json({ success: false, message: "请提供 codesToDelete 数组" });
    }
    const codes = readCodes();
    const deleteSet = new Set(codesToDelete.map(c => c.trim().toUpperCase()));
    const newCodes = codes.filter(c => !deleteSet.has(c.code));
    const deletedCount = codes.length - newCodes.length;
    writeCodes(newCodes);
    res.json({ success: true, message: `成功删除 ${deletedCount} 个卡密` });
  } catch (error) {
    res.status(500).json({ success: false, message: "删除卡密失败" });
  }
});

// 更新卡密 tunnelDays（支持批量）
app.post("/api/admin/codes/update", (req, res) => {
  try {
    const { codesToUpdate, tunnelDays } = req.body;
    if (tunnelDays === undefined || tunnelDays === null) {
      return res.status(400).json({ success: false, message: "请提供 tunnelDays" });
    }
    const codes = readCodes();
    let updatedCount = 0;
    if (codesToUpdate && Array.isArray(codesToUpdate)) {
      const updateSet = new Set(codesToUpdate.map(c => c.trim().toUpperCase()));
      codes.forEach(c => {
        if (updateSet.has(c.code)) { c.tunnelDays = tunnelDays; updatedCount++; }
      });
    } else {
      codes.forEach(c => { c.tunnelDays = tunnelDays; updatedCount++; });
    }
    writeCodes(codes);
    res.json({ success: true, message: `成功更新 ${updatedCount} 个卡密` });
  } catch (error) {
    res.status(500).json({ success: false, message: "更新卡密失败" });
  }
});

// 重置卡密（解除绑定）
app.post("/api/admin/codes/reset", (req, res) => {
  try {
    const { codesToReset } = req.body;
    if (!codesToReset || !Array.isArray(codesToReset)) {
      return res.status(400).json({ success: false, message: "请提供 codesToReset 数组" });
    }
    const codes = readCodes();
    const resetSet = new Set(codesToReset.map(c => c.trim().toUpperCase()));
    let resetCount = 0;
    codes.forEach(c => {
      if (resetSet.has(c.code)) {
        c.active = false;
        c.machineId = null;
        c.activatedAt = null;
        resetCount++;
      }
    });
    writeCodes(codes);
    res.json({ success: true, message: `成功重置 ${resetCount} 个卡密` });
  } catch (error) {
    res.status(500).json({ success: false, message: "重置卡密失败" });
  }
});

// 导出卡密（纯文本，一行一个）
app.get("/api/admin/codes/export", (req, res) => {
  try {
    const { type } = req.query; // all, activated, unused
    const codes = readCodes();
    let filtered = codes;
    if (type === 'activated') filtered = codes.filter(c => c.active);
    else if (type === 'unused') filtered = codes.filter(c => !c.active);
    const text = filtered.map(c => c.code).join('\n');
    res.setHeader('Content-Type', 'text/plain; charset=utf-8');
    res.setHeader('Content-Disposition', `attachment; filename=codes-${type || 'all'}.txt`);
    res.send(text);
  } catch (error) {
    res.status(500).json({ success: false, message: "导出失败" });
  }
});

// 管理页面
app.get("/admin", (req, res) => {
  res.send(getAdminHTML());
});

// 检查穿透权限
app.post("/api/tunnel/check", (req, res) => {
  const { code, machineId } = req.body;
  if (!code || !machineId) {
    return res.json({ success: false, message: "缺少参数" });
  }

  const codes = readCodes();
  const trimmed = code.trim().toUpperCase();
  const entry = codes.find((c) => c.code === trimmed);

  if (!entry) {
    return res.json({ success: false, message: "激活码无效" });
  }

  if (!entry.active || entry.machineId !== machineId) {
    return res.json({ success: false, message: "激活码未激活或设备不匹配" });
  }

  const tunnelDays = entry.tunnelDays || 0;
  if (tunnelDays <= 0) {
    return res.json({ success: false, message: "您的账户暂无内网穿透权限，请联系管理员开通" });
  }

  // 计算过期时间
  const activatedAt = new Date(entry.activatedAt);
  const expiresAt = new Date(activatedAt.getTime() + tunnelDays * 24 * 60 * 60 * 1000);
  const now = new Date();

  if (now > expiresAt) {
    return res.json({ 
      success: false, 
      message: `内网穿透权限已过期（过期时间：${expiresAt.toLocaleString('zh-CN')}）` 
    });
  }

  return res.json({ 
    success: true, 
    message: "验证通过",
    tunnelDays,
    expiresAt: expiresAt.toISOString()
  });
});
// 卡密管理页面 HTML
function getAdminHTML() {
  return `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>卡密管理后台</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #f0f2f5; color: #1d2129; min-height: 100vh; }
  .header { background: #fff; border-bottom: 1px solid #e5e6eb; padding: 16px 24px; display: flex; align-items: center; justify-content: space-between; position: sticky; top: 0; z-index: 100; }
  .header h1 { font-size: 20px; font-weight: 600; color: #1d2129; }
  .header .actions { display: flex; gap: 8px; }
  .container { max-width: 1400px; margin: 0 auto; padding: 24px; }
  .stats { display: grid; grid-template-columns: repeat(3, 1fr); gap: 16px; margin-bottom: 24px; }
  .stat-card { background: #fff; border-radius: 8px; padding: 20px; border: 1px solid #e5e6eb; }
  .stat-card .label { font-size: 14px; color: #86909c; margin-bottom: 8px; }
  .stat-card .value { font-size: 28px; font-weight: 600; }
  .stat-card .value.total { color: #165dff; }
  .stat-card .value.activated { color: #00b42a; }
  .stat-card .value.unused { color: #ff7d00; }
  .toolbar { background: #fff; border-radius: 8px 8px 0 0; padding: 16px; border: 1px solid #e5e6eb; border-bottom: none; display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
  .toolbar .left { display: flex; gap: 8px; align-items: center; flex: 1; flex-wrap: wrap; }
  .toolbar .right { display: flex; gap: 8px; align-items: center; }
  .search-input { padding: 6px 12px; border: 1px solid #c9cdd4; border-radius: 4px; font-size: 14px; width: 240px; outline: none; transition: border-color 0.2s; }
  .search-input:focus { border-color: #165dff; }
  select { padding: 6px 12px; border: 1px solid #c9cdd4; border-radius: 4px; font-size: 14px; outline: none; background: #fff; cursor: pointer; }
  select:focus { border-color: #165dff; }
  .btn { padding: 6px 16px; border-radius: 4px; font-size: 14px; cursor: pointer; border: 1px solid transparent; transition: all 0.2s; display: inline-flex; align-items: center; gap: 4px; white-space: nowrap; }
  .btn-primary { background: #165dff; color: #fff; }
  .btn-primary:hover { background: #4080ff; }
  .btn-success { background: #00b42a; color: #fff; }
  .btn-success:hover { background: #23c343; }
  .btn-warning { background: #ff7d00; color: #fff; }
  .btn-warning:hover { background: #ff9a2e; }
  .btn-danger { background: #f53f3f; color: #fff; }
  .btn-danger:hover { background: #f76560; }
  .btn-outline { background: #fff; color: #4e5969; border-color: #c9cdd4; }
  .btn-outline:hover { border-color: #165dff; color: #165dff; }
  .btn-sm { padding: 2px 8px; font-size: 12px; }
  .table-wrap { background: #fff; border: 1px solid #e5e6eb; border-radius: 0 0 8px 8px; overflow-x: auto; }
  table { width: 100%; border-collapse: collapse; font-size: 14px; }
  thead th { background: #f7f8fa; padding: 12px 16px; text-align: left; font-weight: 500; color: #4e5969; border-bottom: 1px solid #e5e6eb; white-space: nowrap; position: sticky; top: 0; }
  tbody td { padding: 10px 16px; border-bottom: 1px solid #f2f3f5; }
  tbody tr:hover { background: #f7f8fa; }
  .code-text { font-family: 'SF Mono', Monaco, 'Courier New', monospace; font-size: 13px; font-weight: 500; letter-spacing: 0.5px; }
  .badge { display: inline-block; padding: 2px 8px; border-radius: 10px; font-size: 12px; font-weight: 500; }
  .badge-success { background: #e8ffea; color: #00b42a; }
  .badge-default { background: #f2f3f5; color: #86909c; }
  .machine-id { max-width: 180px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 12px; color: #86909c; font-family: monospace; }
  .checkbox { width: 16px; height: 16px; cursor: pointer; accent-color: #165dff; }
  .modal-overlay { display: none; position: fixed; top: 0; left: 0; width: 100%; height: 100%; background: rgba(0,0,0,0.45); z-index: 200; justify-content: center; align-items: center; }
  .modal-overlay.show { display: flex; }
  .modal { background: #fff; border-radius: 8px; padding: 24px; width: 480px; max-width: 90vw; max-height: 80vh; overflow-y: auto; }
  .modal h3 { font-size: 16px; font-weight: 600; margin-bottom: 16px; }
  .modal .form-group { margin-bottom: 16px; }
  .modal label { display: block; font-size: 14px; color: #4e5969; margin-bottom: 6px; }
  .modal input, .modal textarea { width: 100%; padding: 8px 12px; border: 1px solid #c9cdd4; border-radius: 4px; font-size: 14px; outline: none; transition: border-color 0.2s; }
  .modal input:focus, .modal textarea:focus { border-color: #165dff; }
  .modal textarea { resize: vertical; min-height: 100px; font-family: monospace; }
  .modal .form-tip { font-size: 12px; color: #86909c; margin-top: 4px; }
  .modal .modal-footer { display: flex; justify-content: flex-end; gap: 8px; margin-top: 20px; }
  .toast { position: fixed; top: 24px; right: 24px; padding: 12px 20px; border-radius: 4px; font-size: 14px; color: #fff; z-index: 300; animation: fadeIn 0.3s; }
  .toast.success { background: #00b42a; }
  .toast.error { background: #f53f3f; }
  .toast.info { background: #165dff; }
  @keyframes fadeIn { from { opacity: 0; transform: translateY(-10px); } to { opacity: 1; transform: translateY(0); } }
  .empty { text-align: center; padding: 60px 20px; color: #86909c; }
  .pagination { display: flex; align-items: center; justify-content: space-between; padding: 12px 16px; background: #fff; border-top: 1px solid #e5e6eb; }
  .pagination .info { font-size: 13px; color: #86909c; }
  .pagination .pages { display: flex; gap: 4px; align-items: center; }
  .page-btn { padding: 4px 10px; border: 1px solid #c9cdd4; border-radius: 4px; background: #fff; cursor: pointer; font-size: 13px; }
  .page-btn:hover { border-color: #165dff; color: #165dff; }
  .page-btn.active { background: #165dff; color: #fff; border-color: #165dff; }
  .page-btn:disabled { opacity: 0.4; cursor: not-allowed; }
  .selected-bar { background: #e8f3ff; padding: 8px 16px; display: flex; align-items: center; gap: 12px; font-size: 14px; color: #165dff; }
  .selected-bar .count { font-weight: 600; }
</style>
</head>
<body>

<div class="header">
  <h1>卡密管理后台</h1>
  <div class="actions">
    <button class="btn btn-outline" onclick="location.reload()">刷新</button>
  </div>
</div>

<div class="container">
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
    <button class="btn btn-sm btn-warning" onclick="showBatchTunnelModal()">设置穿透天数</button>
    <button class="btn btn-sm btn-outline" onclick="batchReset()">重置绑定</button>
    <button class="btn btn-sm btn-danger" onclick="batchDelete()">批量删除</button>
    <button class="btn btn-sm btn-outline" onclick="clearSelection()">取消选择</button>
  </div>

  <div class="table-wrap">
    <table>
      <thead>
        <tr>
          <th><input type="checkbox" class="checkbox" id="selectAll" onchange="toggleSelectAll()"></th>
          <th>激活码</th>
          <th>状态</th>
          <th>机器码</th>
          <th>激活时间</th>
          <th>穿透天数</th>
          <th>操作</th>
        </tr>
      </thead>
      <tbody id="tableBody"></tbody>
    </table>
    <div id="emptyState" class="empty" style="display:none">暂无数据</div>
  </div>

  <div class="pagination" id="pagination"></div>
</div>

<!-- 添加卡密弹窗 -->
<div class="modal-overlay" id="addModal">
  <div class="modal">
    <h3>添加卡密</h3>
    <div class="form-group">
      <label>添加方式</label>
      <select id="addMode" onchange="toggleAddMode()" style="width:100%;padding:8px 12px;border:1px solid #c9cdd4;border-radius:4px;font-size:14px">
        <option value="random">随机生成</option>
        <option value="custom">自定义输入</option>
      </select>
    </div>
    <div id="randomFields">
      <div class="form-group">
        <label>生成数量</label>
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
      </div>
    </div>
    <div id="customFields" style="display:none">
      <div class="form-group">
        <label>自定义卡密</label>
        <textarea id="customCodes" placeholder="一行一个卡密，格式：XXXX-XXXX-XXXX-XXXX"></textarea>
      </div>
    </div>
    <div class="form-group">
      <label>穿透天数（tunnelDays）</label>
      <input type="number" id="addTunnelDays" value="0" min="0">
      <div class="form-tip">0 表示无穿透权限</div>
    </div>
    <div class="modal-footer">
      <button class="btn btn-outline" onclick="closeModal('addModal')">取消</button>
      <button class="btn btn-primary" onclick="submitAdd()">确认添加</button>
    </div>
  </div>
</div>

<!-- 设置穿透天数弹窗 -->
<div class="modal-overlay" id="tunnelModal">
  <div class="modal">
    <h3>设置穿透天数</h3>
    <div class="form-group">
      <label>穿透天数</label>
      <input type="number" id="tunnelDaysInput" value="30" min="0">
      <div class="form-tip">将为选中的卡密设置穿透天数，0 表示无权限</div>
    </div>
    <div class="modal-footer">
      <button class="btn btn-outline" onclick="closeModal('tunnelModal')">取消</button>
      <button class="btn btn-primary" onclick="submitTunnelDays()">确认</button>
    </div>
  </div>
</div>

<!-- 单个卡密设置穿透天数弹窗 -->
<div class="modal-overlay" id="singleTunnelModal">
  <div class="modal">
    <h3>设置穿透天数</h3>
    <div class="form-group">
      <label>卡密: <span id="singleTunnelCode" class="code-text"></span></label>
    </div>
    <div class="form-group">
      <label>穿透天数</label>
      <input type="number" id="singleTunnelDaysInput" value="30" min="0">
    </div>
    <div class="modal-footer">
      <button class="btn btn-outline" onclick="closeModal('singleTunnelModal')">取消</button>
      <button class="btn btn-primary" onclick="submitSingleTunnelDays()">确认</button>
    </div>
  </div>
</div>

<script>
let allCodes = [];
let selected = new Set();
let currentPage = 1;
const pageSize = 50;

async function fetchCodes() {
  try {
    const res = await fetch('/api/admin/codes');
    const data = await res.json();
    if (data.success) {
      allCodes = data.codes;
      document.getElementById('statTotal').textContent = data.stats.total;
      document.getElementById('statActivated').textContent = data.stats.activated;
      document.getElementById('statUnused').textContent = data.stats.unused;
      renderTable();
    }
  } catch (e) { showToast('加载失败: ' + e.message, 'error'); }
}

function getFiltered() {
  const search = document.getElementById('searchInput').value.trim().toUpperCase();
  const status = document.getElementById('filterStatus').value;
  return allCodes.filter(c => {
    if (status === 'activated' && !c.active) return false;
    if (status === 'unused' && c.active) return false;
    if (search && !c.code.includes(search) && !(c.machineId || '').toUpperCase().includes(search)) return false;
    return true;
  });
}

function renderTable() {
  const filtered = getFiltered();
  const totalPages = Math.max(1, Math.ceil(filtered.length / pageSize));
  if (currentPage > totalPages) currentPage = totalPages;
  const start = (currentPage - 1) * pageSize;
  const pageData = filtered.slice(start, start + pageSize);

  const tbody = document.getElementById('tableBody');
  if (pageData.length === 0) {
    tbody.innerHTML = '';
    document.getElementById('emptyState').style.display = 'block';
  } else {
    document.getElementById('emptyState').style.display = 'none';
    tbody.innerHTML = pageData.map(c => {
      const checked = selected.has(c.code) ? 'checked' : '';
      const statusBadge = c.active
        ? '<span class="badge badge-success">已激活</span>'
        : '<span class="badge badge-default">未使用</span>';
      const machineId = c.machineId ? '<span class="machine-id" title="' + c.machineId + '">' + c.machineId + '</span>' : '<span style="color:#c9cdd4">-</span>';
      const activatedAt = c.activatedAt ? new Date(c.activatedAt).toLocaleString('zh-CN') : '<span style="color:#c9cdd4">-</span>';
      return '<tr>' +
        '<td><input type="checkbox" class="checkbox" ' + checked + ' onchange="toggleSelect(\\'' + c.code + '\\')"></td>' +
        '<td><span class="code-text">' + c.code + '</span></td>' +
        '<td>' + statusBadge + '</td>' +
        '<td>' + machineId + '</td>' +
        '<td style="font-size:13px;color:#4e5969">' + activatedAt + '</td>' +
        '<td><span style="font-weight:500">' + (c.tunnelDays || 0) + '</span> 天</td>' +
        '<td>' +
          '<button class="btn btn-sm btn-outline" onclick="showSingleTunnelModal(\\'' + c.code + '\\',' + (c.tunnelDays||0) + ')">设天数</button> ' +
          (c.active ? '<button class="btn btn-sm btn-warning" onclick="resetSingle(\\'' + c.code + '\\')">重置</button> ' : '') +
          '<button class="btn btn-sm btn-danger" onclick="deleteSingle(\\'' + c.code + '\\')">删除</button>' +
        '</td></tr>';
    }).join('');
  }

  updateSelectedBar();
  renderPagination(filtered.length, totalPages);
}

function renderPagination(total, totalPages) {
  const pg = document.getElementById('pagination');
  let html = '<div class="info">共 ' + total + ' 条</div><div class="pages">';
  html += '<button class="page-btn" onclick="goPage(' + (currentPage - 1) + ')" ' + (currentPage <= 1 ? 'disabled' : '') + '>&lt;</button>';
  const maxShow = 7;
  let startP = Math.max(1, currentPage - 3);
  let endP = Math.min(totalPages, startP + maxShow - 1);
  if (endP - startP < maxShow - 1) startP = Math.max(1, endP - maxShow + 1);
  for (let i = startP; i <= endP; i++) {
    html += '<button class="page-btn ' + (i === currentPage ? 'active' : '') + '" onclick="goPage(' + i + ')">' + i + '</button>';
  }
  html += '<button class="page-btn" onclick="goPage(' + (currentPage + 1) + ')" ' + (currentPage >= totalPages ? 'disabled' : '') + '>&gt;</button>';
  html += '</div>';
  pg.innerHTML = html;
}

function goPage(p) { const filtered = getFiltered(); const tp = Math.max(1, Math.ceil(filtered.length / pageSize)); if (p >= 1 && p <= tp) { currentPage = p; renderTable(); } }
function toggleSelect(code) { if (selected.has(code)) selected.delete(code); else selected.add(code); updateSelectedBar(); }
function toggleSelectAll() {
  const all = document.getElementById('selectAll').checked;
  const filtered = getFiltered();
  const start = (currentPage - 1) * pageSize;
  const pageData = filtered.slice(start, start + pageSize);
  pageData.forEach(c => { if (all) selected.add(c.code); else selected.delete(c.code); });
  renderTable();
}
function clearSelection() { selected.clear(); document.getElementById('selectAll').checked = false; renderTable(); }
function updateSelectedBar() {
  const bar = document.getElementById('selectedBar');
  if (selected.size > 0) { bar.style.display = 'flex'; document.getElementById('selectedCount').textContent = selected.size; }
  else { bar.style.display = 'none'; }
}

function showToast(msg, type = 'success') {
  const t = document.createElement('div');
  t.className = 'toast ' + type;
  t.textContent = msg;
  document.body.appendChild(t);
  setTimeout(() => t.remove(), 3000);
}

function showAddModal() { document.getElementById('addModal').classList.add('show'); }
function showBatchTunnelModal() { document.getElementById('tunnelModal').classList.add('show'); }
function closeModal(id) { document.getElementById(id).classList.remove('show'); }
function toggleAddMode() {
  const mode = document.getElementById('addMode').value;
  document.getElementById('randomFields').style.display = mode === 'random' ? 'block' : 'none';
  document.getElementById('customFields').style.display = mode === 'custom' ? 'block' : 'none';
}

function showSingleTunnelModal(code, days) {
  document.getElementById('singleTunnelCode').textContent = code;
  document.getElementById('singleTunnelDaysInput').value = days;
  document.getElementById('singleTunnelModal').dataset.code = code;
  document.getElementById('singleTunnelModal').classList.add('show');
}

async function submitAdd() {
  const mode = document.getElementById('addMode').value;
  const tunnelDays = parseInt(document.getElementById('addTunnelDays').value) || 0;
  let body = { tunnelDays };
  if (mode === 'random') {
    body.count = parseInt(document.getElementById('addCount').value) || 10;
  } else {
    const text = document.getElementById('customCodes').value.trim();
    if (!text) return showToast('请输入卡密', 'error');
    body.customCodes = text.split('\\n').map(s => s.trim()).filter(Boolean);
  }
  try {
    const res = await fetch('/api/admin/codes', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
    const data = await res.json();
    if (data.success) { showToast(data.message); closeModal('addModal'); fetchCodes(); }
    else showToast(data.message, 'error');
  } catch (e) { showToast('添加失败', 'error'); }
}

async function submitTunnelDays() {
  const days = parseInt(document.getElementById('tunnelDaysInput').value);
  if (isNaN(days)) return showToast('请输入有效天数', 'error');
  try {
    const res = await fetch('/api/admin/codes/update', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ codesToUpdate: [...selected], tunnelDays: days }) });
    const data = await res.json();
    if (data.success) { showToast(data.message); closeModal('tunnelModal'); selected.clear(); fetchCodes(); }
    else showToast(data.message, 'error');
  } catch (e) { showToast('更新失败', 'error'); }
}

async function submitSingleTunnelDays() {
  const code = document.getElementById('singleTunnelModal').dataset.code;
  const days = parseInt(document.getElementById('singleTunnelDaysInput').value);
  if (isNaN(days)) return showToast('请输入有效天数', 'error');
  try {
    const res = await fetch('/api/admin/codes/update', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ codesToUpdate: [code], tunnelDays: days }) });
    const data = await res.json();
    if (data.success) { showToast(data.message); closeModal('singleTunnelModal'); fetchCodes(); }
    else showToast(data.message, 'error');
  } catch (e) { showToast('更新失败', 'error'); }
}

async function batchDelete() {
  if (!confirm('确定要删除选中的 ' + selected.size + ' 个卡密吗？')) return;
  try {
    const res = await fetch('/api/admin/codes/delete', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ codesToDelete: [...selected] }) });
    const data = await res.json();
    if (data.success) { showToast(data.message); selected.clear(); fetchCodes(); }
    else showToast(data.message, 'error');
  } catch (e) { showToast('删除失败', 'error'); }
}

async function batchReset() {
  if (!confirm('确定要重置选中的 ' + selected.size + ' 个卡密的绑定吗？')) return;
  try {
    const res = await fetch('/api/admin/codes/reset', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ codesToReset: [...selected] }) });
    const data = await res.json();
    if (data.success) { showToast(data.message); selected.clear(); fetchCodes(); }
    else showToast(data.message, 'error');
  } catch (e) { showToast('重置失败', 'error'); }
}

async function deleteSingle(code) {
  if (!confirm('确定要删除卡密 ' + code + ' 吗？')) return;
  try {
    const res = await fetch('/api/admin/codes/delete', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ codesToDelete: [code] }) });
    const data = await res.json();
    if (data.success) { showToast(data.message); selected.delete(code); fetchCodes(); }
    else showToast(data.message, 'error');
  } catch (e) { showToast('删除失败', 'error'); }
}

async function resetSingle(code) {
  if (!confirm('确定要重置卡密 ' + code + ' 的绑定吗？')) return;
  try {
    const res = await fetch('/api/admin/codes/reset', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ codesToReset: [code] }) });
    const data = await res.json();
    if (data.success) { showToast(data.message); fetchCodes(); }
    else showToast(data.message, 'error');
  } catch (e) { showToast('重置失败', 'error'); }
}

function toggleExportMenu() {
  const m = document.getElementById('exportMenu');
  m.style.display = m.style.display === 'none' ? 'block' : 'none';
}
document.addEventListener('click', (e) => {
  if (!e.target.closest('#exportMenu') && !e.target.closest('[onclick*="toggleExportMenu"]')) {
    document.getElementById('exportMenu').style.display = 'none';
  }
});

function exportCodes(type) {
  window.open('/api/admin/codes/export?type=' + type, '_blank');
  document.getElementById('exportMenu').style.display = 'none';
}

// 点击遮罩关闭弹窗
document.querySelectorAll('.modal-overlay').forEach(el => {
  el.addEventListener('click', (e) => { if (e.target === el) el.classList.remove('show'); });
});

fetchCodes();
</script>
</body>
</html>`;
}

// 启动服务器
app.listen(PORT, '0.0.0.0', () => {
  console.log(`🚀 Server is running on http://localhost:${PORT}`)
  console.log(`🔌 WebSocket is running on ws://localhost:${PORT}/ws`)
  console.log(`📁 Data directory: ${DATA_DIR}`)
})