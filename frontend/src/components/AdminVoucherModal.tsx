import { useEffect, useState } from 'react';
import { fundAPI } from '../api';
import { ExpenseVoucher } from '../types';

interface Props {
  applicationId: string;
  onClose: () => void;
  onChanged: () => void;
}

const money = (n: number) => `¥${(n || 0).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;

// AdminVoucherModal 平台查看申请下全部凭证（含待核验）并核验。
// 已核验凭证不可重复核验（后端也会以 409 拒绝）。
const AdminVoucherModal = ({ applicationId, onClose, onChanged }: Props) => {
  const [vouchers, setVouchers] = useState<ExpenseVoucher[]>([]);
  const [busy, setBusy] = useState<string>('');
  const [error, setError] = useState('');

  const load = async () => {
    try {
      const res = await fundAPI.getApplication(applicationId);
      setVouchers(res.data.vouchers || []);
    } catch (e) {
      console.error(e);
    }
  };

  useEffect(() => {
    load();
  }, [applicationId]); // eslint-disable-line react-hooks/exhaustive-deps

  const handleCheck = async (id: string) => {
    setError('');
    setBusy(id);
    try {
      await fundAPI.checkVoucher(id);
      await load();
      onChanged();
    } catch (e: any) {
      setError(e.response?.data?.message || '核验失败');
    } finally {
      setBusy('');
    }
  };

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 p-4" onClick={onClose}>
      <div className="bg-white rounded-2xl max-w-2xl w-full max-h-[85vh] overflow-y-auto p-6" onClick={(e) => e.stopPropagation()}>
        <div className="flex justify-between items-center mb-4">
          <h3 className="text-lg font-bold">支出凭证核验</h3>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-600">✕</button>
        </div>

        {error && <div className="text-red-600 text-sm mb-3">{error}</div>}

        {vouchers.length === 0 ? (
          <p className="text-gray-400 text-sm text-center py-8">组织尚未回填凭证</p>
        ) : (
          <div className="space-y-3">
            {vouchers.map((v) => (
              <div key={v.id} className="border border-gray-100 rounded-lg p-4 flex justify-between items-start gap-4">
                <div className="flex-1">
                  <div className="flex items-center gap-2 mb-1">
                    {v.status === 'checked' ? (
                      <span className="px-2 py-0.5 rounded text-xs bg-green-100 text-green-700">已核验</span>
                    ) : (
                      <span className="px-2 py-0.5 rounded text-xs bg-amber-100 text-amber-700">待核验</span>
                    )}
                    <span className="font-medium text-gray-900">{money(v.amount)}</span>
                    <span className="text-sm text-gray-500">{v.category}</span>
                  </div>
                  <div className="text-sm text-gray-600">{v.usage}</div>
                  <div className="text-xs text-gray-400 mt-1">
                    支出日期 {new Date(v.spentAt).toLocaleDateString()}
                    {v.invoiceNo && ` · 发票号 ${v.invoiceNo}`}
                    {v.checkedAt && ` · 核验于 ${new Date(v.checkedAt).toLocaleString()}`}
                  </div>
                  {v.progressNote && <div className="text-xs text-gray-500 mt-1">进展：{v.progressNote}</div>}
                </div>
                {v.status !== 'checked' && (
                  <button
                    onClick={() => handleCheck(v.id)}
                    disabled={busy === v.id}
                    className="bg-green-50 text-green-700 px-3 py-1.5 rounded-lg text-sm hover:bg-green-100 disabled:opacity-50"
                  >
                    {busy === v.id ? '核验中...' : '核验通过'}
                  </button>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
};

export default AdminVoucherModal;
