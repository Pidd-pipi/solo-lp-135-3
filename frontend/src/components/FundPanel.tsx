import { DisbursementOrder, ExpenseVoucher, FundSummary } from '../types';

interface Props {
  summary: FundSummary;
  disbursements: DisbursementOrder[];
  vouchers: ExpenseVoucher[];
}

const money = (n: number) => `¥${(n || 0).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;

// FundPanel 资金透明公示：资金汇总、已审核拨付单、支出凭证。
// 仅展示平台审核通过后的数据，供捐赠人在项目页查看。
const FundPanel = ({ summary, disbursements, vouchers }: Props) => {
  const usedRatio = summary.disbursedAmount > 0
    ? Math.min(100, Math.round((summary.usedAmount / summary.disbursedAmount) * 100))
    : 0;

  return (
    <div className="space-y-8">
      {/* 资金汇总卡片 */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <div className="bg-primary-50 rounded-xl p-4">
          <div className="text-sm text-gray-500 mb-1">已筹资金</div>
          <div className="text-xl font-bold text-primary-700">{money(summary.raisedAmount)}</div>
        </div>
        <div className="bg-blue-50 rounded-xl p-4">
          <div className="text-sm text-gray-500 mb-1">已拨金额</div>
          <div className="text-xl font-bold text-blue-700">{money(summary.disbursedAmount)}</div>
        </div>
        <div className="bg-green-50 rounded-xl p-4">
          <div className="text-sm text-gray-500 mb-1">已用金额</div>
          <div className="text-xl font-bold text-green-700">{money(summary.usedAmount)}</div>
        </div>
        <div className="bg-amber-50 rounded-xl p-4">
          <div className="text-sm text-gray-500 mb-1">剩余可用</div>
          <div className="text-xl font-bold text-amber-700">{money(summary.remainingAmount)}</div>
          {summary.pendingAmount > 0 && (
            <div className="text-xs text-amber-600 mt-1">含待审 {money(summary.pendingAmount)}</div>
          )}
        </div>
      </div>

      <div>
        <div className="flex justify-between text-sm text-gray-500 mb-2">
          <span>拨付资金执行进度</span>
          <span>{usedRatio}%</span>
        </div>
        <div className="w-full bg-gray-200 rounded-full h-2.5">
          <div className="bg-green-500 h-2.5 rounded-full" style={{ width: `${usedRatio}%` }} />
        </div>
      </div>

      {/* 拨付单 */}
      <div>
        <h3 className="text-lg font-semibold text-gray-900 mb-3">平台拨付单（{disbursements.length}）</h3>
        {disbursements.length === 0 ? (
          <p className="text-gray-500 text-center py-6 bg-gray-50 rounded-lg">暂无已审核的拨付记录</p>
        ) : (
          <div className="space-y-3">
            {disbursements.map((d) => (
              <div key={d.id} className="border border-gray-100 rounded-lg p-4">
                <div className="flex justify-between items-start mb-2">
                  <div>
                    <div className="font-medium text-gray-900">拨付单号 {d.orderNo}</div>
                    <div className="text-sm text-gray-500 mt-1">用途：{d.purpose}</div>
                  </div>
                  <div className="text-right">
                    <div className="text-primary-600 font-semibold">{money(d.amount)}</div>
                    <span className={`inline-block mt-1 px-2 py-0.5 rounded text-xs ${
                      d.status === 'paid' ? 'bg-green-100 text-green-700' : 'bg-gray-100 text-gray-600'
                    }`}>
                      {d.status === 'paid' ? '已拨付' : '待拨付'}
                    </span>
                  </div>
                </div>
                <div className="text-xs text-gray-400">
                  生成时间 {new Date(d.createdAt).toLocaleString()}
                  {d.paidAt && ` · 拨付时间 ${new Date(d.paidAt).toLocaleString()}`}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* 支出凭证：仅展示平台已核验的支出 */}
      <div>
        <h3 className="text-lg font-semibold text-gray-900 mb-1">已核验支出凭证（{vouchers.length}）</h3>
        <p className="text-xs text-gray-400 mb-3">仅展示平台核验通过的实际支出；待核验凭证暂不公示，也不计入已用金额。</p>
        {vouchers.length === 0 ? (
          <p className="text-gray-500 text-center py-6 bg-gray-50 rounded-lg">暂无可公示的已核验支出</p>
        ) : (
          <div className="overflow-hidden border border-gray-100 rounded-lg">
            <table className="min-w-full text-sm">
              <thead className="bg-gray-50 text-gray-500">
                <tr>
                  <th className="px-4 py-2 text-left font-medium">支出日期</th>
                  <th className="px-4 py-2 text-left font-medium">类别</th>
                  <th className="px-4 py-2 text-left font-medium">用途说明</th>
                  <th className="px-4 py-2 text-left font-medium">发票号</th>
                  <th className="px-4 py-2 text-right font-medium">金额</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {vouchers.map((v) => (
                  <tr key={v.id}>
                    <td className="px-4 py-2 text-gray-600">{new Date(v.spentAt).toLocaleDateString()}</td>
                    <td className="px-4 py-2 text-gray-900">{v.category}</td>
                    <td className="px-4 py-2 text-gray-600">
                      {v.usage}
                      {v.status === 'checked' && (
                        <span className="ml-2 text-xs text-green-600">已核验</span>
                      )}
                    </td>
                    <td className="px-4 py-2 text-gray-500">{v.invoiceNo || '-'}</td>
                    <td className="px-4 py-2 text-right text-gray-900 font-medium">{money(v.amount)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
};

export default FundPanel;
