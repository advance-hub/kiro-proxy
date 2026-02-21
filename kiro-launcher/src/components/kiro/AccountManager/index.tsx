import React, { useState, useEffect, useCallback, useMemo } from "react";
import { Button, Card, Input, Typography, Tag, Toast, Tooltip, Space, Select, Modal } from "@douyinfe/semi-ui";
import { IconRefresh, IconKey, IconSearch, IconPlus, IconDelete, IconDownload, IconUpload } from "@douyinfe/semi-icons";
import AccountCard from "./AccountCard";
import AddAccountModal from "./AddAccountModal";
import AccountDetailModal from "./AccountDetailModal";
import ImportAccountModal from "./ImportAccountModal";
import EditAccountModal from "./EditAccountModal";

const { Text } = Typography;

const wails = () => {
  if (!window.go?.main?.App) throw new Error("Wails runtime 尚未就绪");
  return window.go.main.App;
};

interface Account {
  id: string;
  email: string;
  label: string;
  status: string;
  addedAt: string;
  provider: string;
  accessToken?: string;
  refreshToken: string;
  expiresAt?: string;
  clientId?: string;
  clientSecret?: string;
  region?: string;
  usageData?: any;
}

export default function AccountManager() {
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [loading, setLoading] = useState("");
  const [refreshingId, setRefreshingId] = useState<string | null>(null);
  const [showAddModal, setShowAddModal] = useState(false);
  const [showImportModal, setShowImportModal] = useState(false);
  const [detailAccount, setDetailAccount] = useState<Account | null>(null);
  const [editLabelAccount, setEditLabelAccount] = useState<Account | null>(null);
  const [searchTerm, setSearchTerm] = useState("");
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [pageSize, setPageSize] = useState(20);
  const [currentPage, setCurrentPage] = useState(1);
  const [batchRefreshing, setBatchRefreshing] = useState(false);
  const [refreshProgress, setRefreshProgress] = useState({ current: 0, total: 0 });
  const [autoRefreshing, setAutoRefreshing] = useState(false);
  const autoRefreshingRef = React.useRef(false);

  const isExpiringSoon = useCallback((account: Account) => {
    if (!account.expiresAt) return true;
    const expiresAt = new Date(account.expiresAt.replace(/\//g, '-'));
    return expiresAt.getTime() - Date.now() < 5 * 60 * 1000; // 5分钟内过期
  }, []);

  const autoRefreshExpired = useCallback(async (accountList: Account[]) => {
    if (autoRefreshingRef.current || accountList.length === 0) return;
    const expiring = accountList.filter(a => isExpiringSoon(a) && a.status !== "已封禁" && a.status !== "Token已失效");
    if (expiring.length === 0) return;

    autoRefreshingRef.current = true;
    setAutoRefreshing(true);
    const updated = [...accountList];

    for (let i = 0; i < expiring.length; i++) {
      try {
        const refreshed = await wails().RefreshAccountToken(expiring[i].id);
        const idx = updated.findIndex(a => a.id === expiring[i].id);
        if (idx !== -1) updated[idx] = refreshed;
      } catch (_) { /* 静默失败 */ }
      if (i < expiring.length - 1) await new Promise(r => setTimeout(r, 500));
    }

    setAccounts(updated);
    autoRefreshingRef.current = false;
    setAutoRefreshing(false);
  }, [isExpiringSoon]);

  const loadAccounts = useCallback(async () => {
    try {
      const list = await wails().GetAccounts();
      setAccounts(list || []);
      return list || [];
    } catch (e) { Toast.error({ content: `获取账号列表失败: ${e}` }); return []; }
  }, []);

  useEffect(() => { 
    loadAccounts().then(list => { if (list.length > 0) autoRefreshExpired(list); });
    const interval = setInterval(async () => {
      if (document.hidden) return;
      const list = await loadAccounts();
      if (list.length > 0) autoRefreshExpired(list);
    }, 5 * 60 * 1000);
    return () => clearInterval(interval);
  }, [loadAccounts, autoRefreshExpired]);

  const filteredAccounts = useMemo(() =>
    accounts.filter(a => a.email.toLowerCase().includes(searchTerm.toLowerCase()) || (a.label || "").toLowerCase().includes(searchTerm.toLowerCase())),
    [accounts, searchTerm]
  );

  const totalPages = Math.ceil(filteredAccounts.length / pageSize) || 1;
  const paginatedAccounts = useMemo(() =>
    filteredAccounts.slice((currentPage - 1) * pageSize, currentPage * pageSize),
    [filteredAccounts, currentPage, pageSize]
  );

  const stats = useMemo(() => {
    const total = accounts.length;
    const active = accounts.filter(a => a.status === "正常" || a.status === "有效").length;
    const banned = accounts.filter(a => a.status === "封禁" || a.status === "已封禁").length;
    return { total, active, banned };
  }, [accounts]);

  const handleSync = async (id: string) => {
    setRefreshingId(id);
    try { 
      const result = await wails().SyncAccount(id); 
      if (result.status === "正常") {
        Toast.success({ content: "账号已刷新" }); 
      } else {
        Toast.warning({ content: `刷新完成，状态: ${result.status}` });
      }
      await loadAccounts(); 
    }
    catch (e) { 
      console.error("刷新失败:", e);
      Toast.error({ content: `刷新失败: ${e}` }); 
      await loadAccounts(); // 刷新列表以显示最新状态
    }
    finally { setRefreshingId(null); }
  };

  const handleSwitch = async (id: string) => {
    setLoading("switch-" + id);
    try { 
      const msg = await wails().SwitchAccount(id); 
      Toast.success({ content: msg }); 
    }
    catch (e) { Toast.error({ content: String(e) }); }
    finally { setLoading(""); }
  };

  const handleDelete = async (id: string) => {
    Modal.confirm({
      title: "删除账号",
      content: "确定要删除这个账号吗？",
      onOk: async () => {
        try { 
          await wails().DeleteAccount(id); 
          Toast.success({ content: "账号已删除" }); 
          await loadAccounts(); 
        }
        catch (e) { Toast.error({ content: String(e) }); }
      }
    });
  };

  const handleBatchDelete = async () => {
    if (selectedIds.length === 0) return;
    Modal.confirm({
      title: "批量删除",
      content: `确定要删除选中的 ${selectedIds.length} 个账号吗？`,
      onOk: async () => {
        try {
          const count = await wails().BatchDeleteAccounts(selectedIds);
          Toast.success({ content: `已删除 ${count} 个账号` });
          setSelectedIds([]);
          await loadAccounts();
        } catch (e) { Toast.error({ content: String(e) }); }
      }
    });
  };

  const handleImportLocal = async () => {
    setLoading("import");
    try { 
      const account = await wails().ImportLocalAccount(); 
      Toast.success({ content: `已导入: ${account.email}` }); 
      await loadAccounts(); 
    }
    catch (e) { Toast.error({ content: String(e) }); }
    finally { setLoading(""); }
  };

  const handleExport = async () => {
    try {
      const msg = await wails().ExportAccountsToFile(selectedIds.length > 0 ? selectedIds : []);
      Toast.success({ content: msg });
    } catch (e) { 
      console.error("导出失败:", e);
      Toast.error({ content: `导出失败: ${e}` }); 
    }
  };

  const handleBatchRefresh = async () => {
    if (batchRefreshing || accounts.length === 0) return;
    setBatchRefreshing(true);
    setRefreshProgress({ current: 0, total: accounts.length });
    
    for (let i = 0; i < accounts.length; i++) {
      setRefreshProgress({ current: i + 1, total: accounts.length });
      try { await wails().SyncAccount(accounts[i].id); } catch {}
      if (i < accounts.length - 1) await new Promise(r => setTimeout(r, 500));
    }
    
    await loadAccounts();
    setBatchRefreshing(false);
    Toast.success({ content: "批量刷新完成" });
  };

  const handleSelectAll = (checked: boolean) => { setSelectedIds(checked ? filteredAccounts.map(a => a.id) : []); };
  const handleSelectOne = (id: string, checked: boolean) => { setSelectedIds(prev => checked ? [...prev, id] : prev.filter(i => i !== id)); };

  return (
    <div style={{ padding: 24, height: "100%", display: "flex", flexDirection: "column", overflow: "hidden" }}>
      {/* Header */}
      <div style={{ marginBottom: 20, flexShrink: 0 }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 16 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
            <Text strong style={{ fontSize: 20 }}>账号管理</Text>
            <Tag size="small" color="blue" type="light">{stats.total} 个账号</Tag>
            {stats.active > 0 && <Tag size="small" color="green" type="light">{stats.active} 正常</Tag>}
            {stats.banned > 0 && <Tag size="small" color="red" type="light">{stats.banned} 封禁</Tag>}
          </div>
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
          <Input prefix={<IconSearch />} placeholder="搜索邮箱/标签" value={searchTerm} onChange={setSearchTerm} style={{ width: 200 }} size="small" />
          {selectedIds.length > 0 && (
            <Button size="small" type="danger" icon={<IconDelete />} onClick={handleBatchDelete}>删除 ({selectedIds.length})</Button>
          )}
          <Tooltip content="批量刷新所有账号">
            <Button size="small" theme="light" icon={<IconRefresh spin={batchRefreshing} />} onClick={handleBatchRefresh} disabled={batchRefreshing}>
              {batchRefreshing ? `${refreshProgress.current}/${refreshProgress.total}` : "刷新全部"}
            </Button>
          </Tooltip>
          <Button size="small" theme="light" icon={<IconUpload />} onClick={() => setShowImportModal(true)}>导入</Button>
          <Button size="small" theme="light" icon={<IconDownload />} onClick={handleExport}>导出{selectedIds.length > 0 ? ` (${selectedIds.length})` : ""}</Button>
          <Button size="small" theme="light" loading={loading === "import"} onClick={handleImportLocal}>导入本地</Button>
          <Button size="small" theme="solid" type="primary" icon={<IconPlus />} onClick={() => setShowAddModal(true)}>添加账号</Button>
        </div>
      </div>

      {/* Select All */}
      {paginatedAccounts.length > 0 && (
        <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 12, flexShrink: 0 }}>
          <input type="checkbox" checked={selectedIds.length === filteredAccounts.length && filteredAccounts.length > 0} onChange={(e) => handleSelectAll(e.target.checked)} style={{ width: 16, height: 16 }} />
          <Text size="small" type="tertiary">{selectedIds.length > 0 ? `已选 ${selectedIds.length}` : "全选"}</Text>
        </div>
      )}

      {/* Account Cards */}
      <div style={{ flex: 1, overflow: "auto", minHeight: 0 }}>
        {paginatedAccounts.length > 0 ? (
          <div style={{ display: "grid", gridTemplateColumns: "repeat(2, 1fr)", gap: 16, maxWidth: 1200 }}>
            {paginatedAccounts.map((account) => (
              <AccountCard
                key={account.id}
                account={account}
                isSelected={selectedIds.includes(account.id)}
                onSelect={(checked) => handleSelectOne(account.id, checked)}
                onSwitch={() => handleSwitch(account.id)}
                onRefresh={() => handleSync(account.id)}
                onDelete={() => handleDelete(account.id)}
                onViewDetail={() => setDetailAccount(account)}
                onEditLabel={() => setEditLabelAccount(account)}
                refreshing={refreshingId === account.id}
                switching={loading === "switch-" + account.id}
              />
            ))}
            {/* Add Card */}
            <div onClick={() => setShowAddModal(true)} style={{ minHeight: 280, borderRadius: 12, border: "2px dashed var(--semi-color-border)", display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", gap: 12, cursor: "pointer", transition: "all 0.2s" }}>
              <div style={{ width: 48, height: 48, borderRadius: "50%", background: "var(--semi-color-fill-0)", display: "flex", alignItems: "center", justifyContent: "center" }}>
                <IconPlus style={{ fontSize: 24, color: "var(--semi-color-text-2)" }} />
              </div>
              <Text type="tertiary" size="small">添加账号</Text>
            </div>
          </div>
        ) : (
          <Card bodyStyle={{ padding: "60px 20px", textAlign: "center" }} style={{ borderRadius: 12 }}>
            <IconKey style={{ fontSize: 48, color: "var(--semi-color-text-3)", marginBottom: 16 }} />
            <Text type="tertiary" style={{ display: "block", marginBottom: 16 }}>还没有账号，点击上方按钮添加</Text>
            <Space>
              <Button theme="light" onClick={handleImportLocal} loading={loading === "import"}>导入本地账号</Button>
              <Button theme="light" onClick={() => setShowImportModal(true)}>导入 JSON</Button>
              <Button theme="solid" type="primary" onClick={() => setShowAddModal(true)}>手动添加</Button>
            </Space>
          </Card>
        )}
      </div>

      {/* Pagination */}
      {filteredAccounts.length > pageSize && (
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginTop: 16, paddingTop: 16, borderTop: "1px solid var(--semi-color-border)", flexShrink: 0 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
            <Text size="small" type="tertiary">每页</Text>
            <Select size="small" value={pageSize} onChange={(v) => { setPageSize(v as number); setCurrentPage(1); }} style={{ width: 70 }}>
              <Select.Option value={10}>10</Select.Option>
              <Select.Option value={20}>20</Select.Option>
              <Select.Option value={50}>50</Select.Option>
            </Select>
            <Text size="small" type="tertiary">共 {filteredAccounts.length} 条</Text>
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 4 }}>
            <Button size="small" disabled={currentPage === 1} onClick={() => setCurrentPage(1)}>首页</Button>
            <Button size="small" disabled={currentPage === 1} onClick={() => setCurrentPage(p => p - 1)}>上一页</Button>
            <Text size="small" style={{ padding: "0 12px" }}>{currentPage} / {totalPages}</Text>
            <Button size="small" disabled={currentPage === totalPages} onClick={() => setCurrentPage(p => p + 1)}>下一页</Button>
            <Button size="small" disabled={currentPage === totalPages} onClick={() => setCurrentPage(totalPages)}>末页</Button>
          </div>
        </div>
      )}

      {/* Modals */}
      {showAddModal && <AddAccountModal onClose={() => setShowAddModal(false)} onSuccess={loadAccounts} />}
      {showImportModal && <ImportAccountModal onClose={() => setShowImportModal(false)} onSuccess={loadAccounts} />}
      {detailAccount && <AccountDetailModal account={detailAccount} onClose={() => { setDetailAccount(null); loadAccounts(); }} />}
      {editLabelAccount && <EditAccountModal account={editLabelAccount} onClose={() => setEditLabelAccount(null)} onSuccess={loadAccounts} />}
    </div>
  );
}
