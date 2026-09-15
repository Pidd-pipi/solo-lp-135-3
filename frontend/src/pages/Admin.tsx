import { useState, useEffect } from 'react';
import { Navigate } from 'react-router-dom';
import { useAuth } from '../context/AuthContext';
import { adminAPI, fundAPI } from '../api';
import { Project, Organization, FundApplication } from '../types';
import AdminVoucherModal from '../components/AdminVoucherModal';

const money = (n: number) => `¥${(n || 0).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;

const appStatusBadge = (s: string) => {
  const map: Record<string, { text: string; cls: string }> = {
    pending: { text: '待审核', cls: 'bg-amber-100 text-amber-700' },
    approved: { text: '已通过', cls: 'bg-green-100 text-green-700' },
    rejected: { text: '已驳回', cls: 'bg-red-100 text-red-700' },
  };
  const m = map[s] || { text: s, cls: 'bg-gray-100 text-gray-600' };
  return <span className={`px-2 py-0.5 rounded text-xs ${m.cls}`}>{m.text}</span>;
};

const Admin = () => {
  const { user } = useAuth();
  const [activeTab, setActiveTab] = useState<'projects' | 'organizations' | 'funds' | 'fundsAll'>('projects');
  const [pendingProjects, setPendingProjects] = useState<Project[]>([]);
  const [pendingOrgs, setPendingOrgs] = useState<Organization[]>([]);
  const [pendingApps, setPendingApps] = useState<FundApplication[]>([]);
  const [allApps, setAllApps] = useState<FundApplication[]>([]);
  const [commentMap, setCommentMap] = useState<Record<string, string>>({});
  const [voucherAppId, setVoucherAppId] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (user?.role === 'admin') {
      loadPendingItems();
    }
  }, [user]);

  const loadPendingItems = async () => {
    try {
      const [projectsRes, orgsRes, appsRes] = await Promise.all([
        adminAPI.getPendingProjects(),
        adminAPI.getPendingOrganizations(),
        fundAPI.getPendingApplications({ page: 1, page_size: 100 }),
      ]);
      setPendingProjects(projectsRes.data.projects);
      setPendingOrgs(orgsRes.data.organizations);
      setPendingApps(appsRes.data.applications || []);
    } catch (error) {
      console.error('加载待审核项失败:', error);
    } finally {
      setLoading(false);
    }
  };

  const loadAllApps = async () => {
    try {
      const res = await fundAPI.getAllApplications({ page: 1, page_size: 100 });
      setAllApps(res.data.applications || []);
    } catch (error) {
      console.error('加载资金申请追溯失败:', error);
    }
  };

  useEffect(() => {
    if (activeTab === 'fundsAll') loadAllApps();
  }, [activeTab]);

  const handleReviewProject = async (projectId: string, status: 'approved' | 'rejected') => {
    try {
      await adminAPI.reviewProject(projectId, { status, comment: '' });
      alert(`项目已${status === 'approved' ? '通过' : '拒绝'}`);
      loadPendingItems();
    } catch (error) {
      alert('操作失败');
    }
  };

  const handleReviewOrg = async (orgId: string, status: 'approved' | 'rejected') => {
    try {
      await adminAPI.reviewOrganization(orgId, { status, comment: '' });
      alert(`组织已${status === 'approved' ? '通过' : '拒绝'}`);
      loadPendingItems();
    } catch (error) {
      alert('操作失败');
    }
  };

  const handleReviewFund = async (appId: string, approve: boolean) => {
    const comment = commentMap[appId] || '';
    try {
      await fundAPI.reviewApplication(appId, { approve, comment });
      alert(approve ? '已通过并生成拨付单' : '已驳回，额度已释放');
      setCommentMap((m) => ({ ...m, [appId]: '' }));
      loadPendingItems();
    } catch (error: any) {
      alert(error.response?.data?.message || '审核失败');
    }
  };

  if (!user || user.role !== 'admin') {
    return <Navigate to="/" />;
  }

  const categoryMap: Record<string, string> = {
    education: '助学',
    elderly: '助老',
    medical: '医疗',
    disaster: '救灾',
    environment: '环保',
    other: '其他',
  };

  return (
    <div>
      <h1 className="text-3xl font-bold text-gray-900 mb-8">管理后台</h1>

      <div className="flex gap-4 mb-8">
        <button
          onClick={() => setActiveTab('projects')}
          className={`px-6 py-2 rounded-lg font-medium ${
            activeTab === 'projects'
              ? 'bg-primary-600 text-white'
              : 'bg-white text-gray-600 border border-gray-200'
          }`}
        >
          待审核项目 ({pendingProjects.length})
        </button>
        <button
          onClick={() => setActiveTab('organizations')}
          className={`px-6 py-2 rounded-lg font-medium ${
            activeTab === 'organizations'
              ? 'bg-primary-600 text-white'
              : 'bg-white text-gray-600 border border-gray-200'
          }`}
        >
          待审核组织 ({pendingOrgs.length})
        </button>
        <button
          onClick={() => setActiveTab('funds')}
          className={`px-6 py-2 rounded-lg font-medium ${
            activeTab === 'funds'
              ? 'bg-primary-600 text-white'
              : 'bg-white text-gray-600 border border-gray-200'
          }`}
        >
          资金拨付审核 ({pendingApps.length})
        </button>
        <button
          onClick={() => setActiveTab('fundsAll')}
          className={`px-6 py-2 rounded-lg font-medium ${
            activeTab === 'fundsAll'
              ? 'bg-primary-600 text-white'
              : 'bg-white text-gray-600 border border-gray-200'
          }`}
        >
          资金追溯
        </button>
      </div>

      {loading ? (
        <div className="text-center py-20">加载中...</div>
      ) : activeTab === 'projects' ? (
        <div className="bg-white rounded-2xl shadow-sm overflow-hidden">
          {pendingProjects.length === 0 ? (
            <div className="text-center py-12 text-gray-500">暂无待审核项目</div>
          ) : (
            <div className="divide-y divide-gray-100">
              {pendingProjects.map((project) => (
                <div key={project.id} className="p-6">
                  <div className="flex justify-between items-start">
                    <div className="flex-1">
                      <div className="flex items-center gap-3 mb-2">
                        <span className="px-2 py-1 bg-blue-100 text-blue-700 rounded text-xs font-medium">
                          {categoryMap[project.category]}
                        </span>
                        <span className="text-sm text-gray-500">
                          发布时间：{new Date(project.createdAt).toLocaleDateString()}
                        </span>
                      </div>
                      <h3 className="text-lg font-semibold text-gray-900 mb-2">{project.title}</h3>
                      <p className="text-gray-600 mb-2">{project.description}</p>
                      <div className="text-sm text-gray-500">
                        目标金额：¥{project.targetAmount.toLocaleString()}
                      </div>
                      <div className="text-sm text-gray-500">
                        执行计划：{project.executionPlan}
                      </div>
                    </div>
                    <div className="flex gap-2 ml-6">
                      <button
                        onClick={() => handleReviewProject(project.id, 'approved')}
                        className="px-4 py-2 bg-green-50 text-green-600 rounded-lg hover:bg-green-100"
                      >
                        通过
                      </button>
                      <button
                        onClick={() => handleReviewProject(project.id, 'rejected')}
                        className="px-4 py-2 bg-red-50 text-red-600 rounded-lg hover:bg-red-100"
                      >
                        拒绝
                      </button>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      ) : activeTab === 'organizations' ? (
        <div className="bg-white rounded-2xl shadow-sm overflow-hidden">
          {pendingOrgs.length === 0 ? (
            <div className="text-center py-12 text-gray-500">暂无待审核组织</div>
          ) : (
            <div className="divide-y divide-gray-100">
              {pendingOrgs.map((org) => (
                <div key={org.id} className="p-6">
                  <div className="flex justify-between items-start">
                    <div className="flex-1">
                      <h3 className="text-lg font-semibold text-gray-900 mb-2">{org.name}</h3>
                      <p className="text-gray-600 mb-2">{org.description}</p>
                      <div className="grid grid-cols-2 gap-4 text-sm text-gray-500">
                        <div>执照编号：{org.licenseNumber || '-'}</div>
                        <div>联系人：{org.contactPerson || '-'}</div>
                        <div>联系电话：{org.contactPhone || '-'}</div>
                        <div>地址：{org.address || '-'}</div>
                      </div>
                    </div>
                    <div className="flex gap-2 ml-6">
                      <button
                        onClick={() => handleReviewOrg(org.id, 'approved')}
                        className="px-4 py-2 bg-green-50 text-green-600 rounded-lg hover:bg-green-100"
                      >
                        通过
                      </button>
                      <button
                        onClick={() => handleReviewOrg(org.id, 'rejected')}
                        className="px-4 py-2 bg-red-50 text-red-600 rounded-lg hover:bg-red-100"
                      >
                        拒绝
                      </button>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      ) : activeTab === 'funds' ? (
        <div className="bg-white rounded-2xl shadow-sm overflow-hidden">
          {pendingApps.length === 0 ? (
            <div className="text-center py-12 text-gray-500">暂无待审核用款申请</div>
          ) : (
            <div className="divide-y divide-gray-100">
              {pendingApps.map((a) => (
                <div key={a.id} className="p-6">
                  <div className="flex justify-between items-start gap-4">
                    <div className="flex-1">
                      <div className="flex items-center gap-3 mb-2">
                        {appStatusBadge(a.status)}
                        <span className="text-sm text-gray-500">批次 {a.batchNo}</span>
                        <span className="text-sm text-gray-500">
                          提交于 {new Date(a.createdAt).toLocaleString()}
                        </span>
                      </div>
                      <h3 className="text-lg font-semibold text-gray-900 mb-1">
                        {a.project?.title || `项目 #${a.projectId}`}
                      </h3>
                      <p className="text-gray-600 mb-2">用途：{a.purpose}</p>
                      <div className="text-sm text-gray-500">申请金额：{money(a.amount)}</div>
                      <input
                        type="text"
                        placeholder="审核意见（可选）"
                        value={commentMap[a.id] || ''}
                        onChange={(e) => setCommentMap((m) => ({ ...m, [a.id]: e.target.value }))}
                        className="mt-3 w-full border border-gray-200 rounded-lg px-3 py-2 text-sm"
                      />
                    </div>
                    <div className="flex flex-col gap-2">
                      <button
                        onClick={() => handleReviewFund(a.id, true)}
                        className="px-4 py-2 bg-green-50 text-green-600 rounded-lg hover:bg-green-100"
                      >
                        通过并拨付
                      </button>
                      <button
                        onClick={() => handleReviewFund(a.id, false)}
                        className="px-4 py-2 bg-red-50 text-red-600 rounded-lg hover:bg-red-100"
                      >
                        驳回
                      </button>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      ) : (
        <div className="bg-white rounded-2xl shadow-sm overflow-hidden">
          <div className="px-6 py-4 border-b text-sm text-gray-500">全量用款申请追溯（状态、审核意见与发生时间）</div>
          {allApps.length === 0 ? (
            <div className="text-center py-12 text-gray-500">暂无记录</div>
          ) : (
            <div className="overflow-x-auto">
              <table className="min-w-full text-sm">
                <thead className="bg-gray-50 text-gray-500">
                  <tr>
                    <th className="px-4 py-3 text-left font-medium">项目</th>
                    <th className="px-4 py-3 text-left font-medium">金额/用途</th>
                    <th className="px-4 py-3 text-left font-medium">状态</th>
                    <th className="px-4 py-3 text-left font-medium">拨付单</th>
                    <th className="px-4 py-3 text-left font-medium">审核意见/时间</th>
                    <th className="px-4 py-3 text-left font-medium">凭证</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-100">
                  {allApps.map((a) => (
                    <tr key={a.id}>
                      <td className="px-4 py-3 text-gray-900">{a.project?.title || `#${a.projectId}`}</td>
                      <td className="px-4 py-3">
                        <div className="font-medium text-gray-900">{money(a.amount)}</div>
                        <div className="text-gray-500 text-xs">{a.purpose}</div>
                      </td>
                      <td className="px-4 py-3">{appStatusBadge(a.status)}</td>
                      <td className="px-4 py-3 text-gray-600 text-xs">{a.order?.orderNo || '-'}</td>
                      <td className="px-4 py-3 text-gray-500 text-xs">
                        {a.reviewComment || '-'}
                        {a.reviewedAt && <div>{new Date(a.reviewedAt).toLocaleString()}</div>}
                        {!a.reviewedAt && a.createdAt && <div>提交于 {new Date(a.createdAt).toLocaleString()}</div>}
                      </td>
                      <td className="px-4 py-3">
                        {a.status === 'approved' ? (
                          <button
                            onClick={() => setVoucherAppId(a.id)}
                            className="text-primary-600 hover:underline text-xs"
                          >
                            查看/核验凭证
                          </button>
                        ) : (
                          <span className="text-gray-300 text-xs">-</span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {voucherAppId && (
        <AdminVoucherModal
          applicationId={voucherAppId}
          onClose={() => setVoucherAppId(null)}
          onChanged={() => {
            loadAllApps();
          }}
        />
      )}
    </div>
  );
};

export default Admin;
