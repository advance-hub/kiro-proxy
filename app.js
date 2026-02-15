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
// 启动服务器
app.listen(PORT, () => {
  console.log(`🚀 Server is running on http://localhost:${PORT}`)
  console.log(`🔌 WebSocket is running on ws://localhost:${PORT}/ws`)
  console.log(`📁 Data directory: ${DATA_DIR}`)
})