import { useState, useEffect, useCallback } from 'react';
import { fundAPI } from '../api';
import { FundApplication, ExpenseVoucher } from '../types';

interface Props {
  projectId: string;
  settled: boolean;
  onChanged: () => void;
}

const money = (n: number) => `¥${(n || 0).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;

const statusBadge = (s: string) => {
  const map: Record<string, { text: string; cls: string }> = {
    pending: { text: '待审核', cls: 'bg-amber-100 text-amber-700' },
    approved: { text: '已通过', cls: 'bg-green-100 text-green-700' },
    rejected: { text: '已驳回', cls: 'bg-red-100 text-red-700' },
  };
  const m = map[s] || { text: s, cls: 'bg-gray-100 text-gray-600' };
  return <span className={`px-2 py-0.5 rounded text-xs ${m.cls}`}>{m.text}</span>;
};

// OrgFundManager 组织对单个项目的资金管理：分批申请、回填凭证、结项。
const OrgFundManager = ({ projectId, settled, onChanged }: Props) => {
  const [apps, setApps] = useState<FundApplication[]>([]);
  const [amount, setAmount] = useState('');
  const [purpose, setPurpose] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');
  const [voucherApp, setVoucherApp] = useState<FundApplication | null>(null);
  const [vouchers, setVouchers] = useState<ExpenseVoucher[]>([]);
  const [vForm, setVForm] = useState({ amount: '', category: '', usage: '', invoiceNo: '', spentAt: '', progressNote: '' });

  const loadApps = useCallback(async () => {
    try {
      const res = await fundAPI.getMyApplications({ page: 1, page_size: 100 });
      const list: FundApplication[] = res.data.applications || [];
      setApps(list.filter((a) => String(a.projectId) === String(projectId)));
    } catch (e) {
      console.error('加载用款申请失败', e);
    }
  }, [projectId]);

  useEffect(() => {
    loadApps();
  }, [loadApps]);

  const handleApply = async () => {
    setError('');
    const amt = parseFloat(amount);
    if (!amt || amt <= 0) {
      setError('请输入大于 0 的申请金额');
      return;
    }
    if (!purpose.trim()) {
      setError('请填写资金用途说明');
      return;
    }
    setSubmitting(true);
    try {
      await fundAPI.apply(projectId, { amount: amt, purpose: purpose.trim() });
      setAmount('');
      setPurpose('');
      await loadApps();
      onChanged();
    } catch (e: any) {
      setError(e.response?.data?.message || '申请失败');
    } finally {
      setSubmitting(false);
    }
  };

  const handleSettle = async () => {
    if (!window.confirm('结项后将无法再申请或接收拨付，确认结项？')) return;
    try {
      await fundAPI.settle(projectId);
      alert('项目已结项');
      onChanged();
    } catch (e: any) {
      alert(e.response?.data?.message || '结项失败');
    }
  };

  const openVoucher = async (app: FundApplication) => {
    setVoucherApp(app);
    setError('');
    try {
      const res = await fundAPI.getApplication(app.id);
      setVouchers(res.data.vouchers || []);
    } catch (e) {
      console.error(e);
    }
  };

  const submitVoucher = async () => {
    setError('');
    const amt = parseFloat(vForm.amount);
    if (!amt || amt <= 0) return setError('请输入支出金额');
    if (!vForm.category.trim()) return setError('请填写支出类别');
    if (!vForm.usage.trim()) return setError('请填写用途说明');
    try {
      await fundAPI.addVoucher(voucherApp!.id, {
        amount: amt,
        category: vForm.category.trim(),
        usage: vForm.usage.trim(),
        invoiceNo: vForm.invoiceNo.trim(),
        spentAt: vForm.spentAt || undefined,
        progressNote: vForm.progressNote.trim(),
      });
      setVForm({ amount: '', category: '', usage: '', invoiceNo: '', spentAt: '', progressNote: '' });
      const res = await fundAPI.getApplication(voucherApp!.id);
      setVouchers(res.data.vouchers || []);
      loadApps();
      onChanged();
    } catch (e: any) {
      setError(e.response?.data?.message || '凭证回填失败');
    }
  };

  return (
    <div className="space-y-6">
      {settled && (
        <div className="bg-gray-100 text-gray-600 rounded-lg p-3 text-sm">该项目已结项，不可再提交申请或接收拨付。</div>
      )}

      {/* 提交用款申请 */}
      {!settled && (
        <div className="border border-primary-100 rounded-xl p-4 bg-primary-50/40">
          <h4 className="font-semibold text-gray-900 mb-3">提交分批用款申请</h4>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
            <input
              type="number"
              placeholder="申请金额（元）"
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              className="border border-gray-200 rounded-lg px-3 py-2"
            />
            <input
              type="text"
              placeholder="资金用途，如：采购课外读物"
              value={purpose}
              onChange={(e) => setPurpose(e.target.value)}
              className="border border-gray-200 rounded-lg px-3 py-2 md:col-span-2"
            />
          </div>
          {error && <div className="text-red-600 text-sm mt-2">{error}</div>}
          <div className="mt-3 flex items-center gap-3">
            <button
              onClick={handleApply}
              disabled={submitting}
              className="bg-primary-600 text-white px-4 py-2 rounded-lg hover:bg-primary-700 disabled:opacity-50"
            >
              {submitting ? '提交中...' : '提交申请'}
            </button>
            <button onClick={handleSettle} className="text-gray-600 border border-gray-300 px-4 py-2 rounded-lg hover:bg-gray-50">
              项目结项
            </button>
            <span className="text-xs text-gray-400">待审核与已批金额合计不能超过已筹资金</span>
          </div>
        </div>
      )}

      {/* 本项目申请列表 */}
      <div>
        <h4 className="font-semibold text-gray-900 mb-3">用款申请与拨付单（{apps.length}）</h4>
        {apps.length === 0 ? (
          <p className="text-gray-500 text-sm bg-gray-50 rounded-lg p-4">尚未提交用款申请</p>
        ) : (
          <div className="space-y-2">
            {apps.map((a) => (
              <div key={a.id} className="border border-gray-100 rounded-lg p-3 flex justify-between items-center">
                <div>
                  <div className="flex items-center gap-2">
                    {statusBadge(a.status)}
                    <span className="font-medium text-gray-900">{money(a.amount)}</span>
                    {a.order && <span className="text-xs text-gray-400">拨付单 {a.order.orderNo}</span>}
                  </div>
                  <div className="text-sm text-gray-500 mt-1">{a.purpose}</div>
                  <div className="text-xs text-gray-400 mt-1">
                    批次 {a.batchNo} · 提交于 {new Date(a.createdAt).toLocaleString()}
                    {a.reviewedAt && ` · 审核于 ${new Date(a.reviewedAt).toLocaleString()}`}
                    {a.reviewComment && ` · 意见：${a.reviewComment}`}
                  </div>
                </div>
                {a.status === 'approved' && (
                  <button onClick={() => openVoucher(a)} className="bg-green-50 text-green-700 px-3 py-1.5 rounded-lg text-sm hover:bg-green-100">
                    回填凭证
                  </button>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      {/* 凭证回填弹层 */}
      {voucherApp && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 p-4" onClick={() => setVoucherApp(null)}>
          <div className="bg-white rounded-2xl max-w-2xl w-full max-h-[90vh] overflow-y-auto p-6" onClick={(e) => e.stopPropagation()}>
            <div className="flex justify-between items-center mb-4">
              <h3 className="text-lg font-bold">回填支出凭证 · {money(voucherApp.amount)}</h3>
              <button onClick={() => setVoucherApp(null)} className="text-gray-400 hover:text-gray-600">✕</button>
            </div>

            <div className="grid grid-cols-2 gap-3 mb-3">
              <input className="border rounded-lg px-3 py-2" placeholder="支出金额" type="number" value={vForm.amount} onChange={(e) => setVForm({ ...vForm, amount: e.target.value })} />
              <input className="border rounded-lg px-3 py-2" placeholder="支出类别（物资/物流等）" value={vForm.category} onChange={(e) => setVForm({ ...vForm, category: e.target.value })} />
              <input className="border rounded-lg px-3 py-2 col-span-2" placeholder="用途说明" value={vForm.usage} onChange={(e) => setVForm({ ...vForm, usage: e.target.value })} />
              <input className="border rounded-lg px-3 py-2" placeholder="发票号（可选）" value={vForm.invoiceNo} onChange={(e) => setVForm({ ...vForm, invoiceNo: e.target.value })} />
              <input className="border rounded-lg px-3 py-2" type="date" value={vForm.spentAt} onChange={(e) => setVForm({ ...vForm, spentAt: e.target.value })} />
              <textarea className="border rounded-lg px-3 py-2 col-span-2" rows={2} placeholder="执行进展说明（可选）" value={vForm.progressNote} onChange={(e) => setVForm({ ...vForm, progressNote: e.target.value })} />
            </div>
            {error && <div className="text-red-600 text-sm mb-2">{error}</div>}
            <button onClick={submitVoucher} className="bg-primary-600 text-white px-4 py-2 rounded-lg hover:bg-primary-700">提交凭证</button>

            <div className="mt-5">
              <h4 className="font-semibold text-sm text-gray-700 mb-2">已回填凭证（{vouchers.length}）</h4>
              {vouchers.length === 0 ? (
                <p className="text-gray-400 text-sm">暂无</p>
              ) : (
                <table className="min-w-full text-sm">
                  <thead className="text-gray-400">
                    <tr><th className="text-left py-1">日期</th><th className="text-left">类别</th><th className="text-left">用途</th><th className="text-right">金额</th></tr>
                  </thead>
                  <tbody>
                    {vouchers.map((v) => (
                      <tr key={v.id} className="border-t">
                        <td className="py-1">{new Date(v.spentAt).toLocaleDateString()}</td>
                        <td>{v.category}</td>
                        <td className="text-gray-600">{v.usage}</td>
                        <td className="text-right">{money(v.amount)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
};

export default OrgFundManager;
