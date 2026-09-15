import { useState, useEffect } from 'react';
import { useParams, useNavigate, Link } from 'react-router-dom';
import { projectAPI, donationAPI, fundAPI } from '../api';
import { Project, Donation, ProjectUpdate, DisbursementOrder, ExpenseVoucher, FundSummary } from '../types';
import { useAuth } from '../context/AuthContext';
import DonationModal from '../components/DonationModal';
import FundPanel from '../components/FundPanel';
import OrgFundManager from '../components/OrgFundManager';

const ProjectDetail = () => {
  const { id } = useParams<{ id: string }>();
  const { user } = useAuth();
  const navigate = useNavigate();
  const [project, setProject] = useState<Project | null>(null);
  const [donations, setDonations] = useState<Donation[]>([]);
  const [updates, setUpdates] = useState<ProjectUpdate[]>([]);
  const [showDonationModal, setShowDonationModal] = useState(false);
  const [loading, setLoading] = useState(true);
  const [activeTab, setActiveTab] = useState<'detail' | 'updates' | 'donations' | 'funds'>('detail');
  const [funds, setFunds] = useState<{ orders: DisbursementOrder[]; vouchers: ExpenseVoucher[]; summary: FundSummary } | null>(null);

  useEffect(() => {
    if (id) loadProject();
  }, [id]);

  const loadProject = async () => {
    try {
      const response = await projectAPI.getProject(id!);
      setProject(response.data.project);
      setDonations(response.data.donations);
      setUpdates(response.data.updates);
    } catch (error) {
      console.error('加载项目失败:', error);
    } finally {
      setLoading(false);
    }
  };

  const loadFunds = async () => {
    if (!user || !id) return;
    try {
      const res = await fundAPI.getPublicFunds(id);
      setFunds({
        orders: res.data.disbursements || [],
        vouchers: res.data.vouchers || [],
        summary: res.data.summary,
      });
    } catch (error) {
      console.error('加载资金透明信息失败:', error);
    }
  };

  useEffect(() => {
    if (activeTab === 'funds' && user && id && !funds) loadFunds();
  }, [activeTab, user, id]); // eslint-disable-line react-hooks/exhaustive-deps

  const handleDonateClick = () => {
    if (!user) {
      navigate('/login');
      return;
    }
    setShowDonationModal(true);
  };

  const categoryMap: Record<string, string> = {
    education: '助学',
    elderly: '助老',
    medical: '医疗',
    disaster: '救灾',
    environment: '环保',
    other: '其他',
  };

  const isOwnerOrg = user?.role === 'org' && project?.organization?.userId === user.id;

  if (loading) return <div className="text-center py-20">加载中...</div>;
  if (!project) return <div className="text-center py-20">项目不存在</div>;

  return (
    <div>
      <div className="grid grid-cols-3 gap-8">
        <div className="col-span-2">
          <div className="bg-white rounded-2xl shadow-sm overflow-hidden mb-6">
            <div className="h-80 bg-gradient-to-br from-primary-400 to-primary-600" />
            <div className="p-8">
              <div className="flex items-center gap-3 mb-4">
                <span className="px-3 py-1 bg-primary-100 text-primary-700 rounded-full text-sm font-medium">
                  {categoryMap[project.category]}
                </span>
                {project.status === 'completed' && (
                  <span className="px-3 py-1 bg-gray-100 text-gray-700 rounded-full text-sm font-medium">
                    已完成
                  </span>
                )}
                {project.settledAt && (
                  <span className="px-3 py-1 bg-red-50 text-red-600 rounded-full text-sm font-medium">
                    已结项
                  </span>
                )}
              </div>
              <h1 className="text-3xl font-bold text-gray-900 mb-4">{project.title}</h1>
              <div className="flex items-center gap-4 text-gray-500 mb-6">
                <span>发起机构：{project.organization?.name}</span>
                <span>发布时间：{new Date(project.createdAt).toLocaleDateString()}</span>
              </div>

              <div className="border-b border-gray-100 mb-6">
                <div className="flex gap-8">
                  <button
                    onClick={() => setActiveTab('detail')}
                    className={`pb-4 font-medium ${activeTab === 'detail' ? 'text-primary-600 border-b-2 border-primary-600' : 'text-gray-500'}`}
                  >
                    项目详情
                  </button>
                  <button
                    onClick={() => setActiveTab('updates')}
                    className={`pb-4 font-medium ${activeTab === 'updates' ? 'text-primary-600 border-b-2 border-primary-600' : 'text-gray-500'}`}
                  >
                    项目动态 ({updates.length})
                  </button>
                  <button
                    onClick={() => setActiveTab('donations')}
                    className={`pb-4 font-medium ${activeTab === 'donations' ? 'text-primary-600 border-b-2 border-primary-600' : 'text-gray-500'}`}
                  >
                    捐赠记录 ({donations.length})
                  </button>
                  <button
                    onClick={() => user ? setActiveTab('funds') : navigate('/login')}
                    className={`pb-4 font-medium ${activeTab === 'funds' ? 'text-primary-600 border-b-2 border-primary-600' : 'text-gray-500'}`}
                  >
                    资金透明
                  </button>
                </div>
              </div>

              {activeTab === 'detail' && (
                <div className="prose max-w-none">
                  <h3 className="text-xl font-semibold mb-3">项目介绍</h3>
                  <p className="text-gray-600 mb-6">{project.description}</p>
                  <h3 className="text-xl font-semibold mb-3">执行计划</h3>
                  <p className="text-gray-600 whitespace-pre-wrap">{project.executionPlan}</p>
                </div>
              )}

              {activeTab === 'updates' && (
                <div className="space-y-6">
                  {isOwnerOrg && (
                    <Link
                      to={`/create-update/${project.id}`}
                      className="inline-flex items-center gap-2 bg-primary-600 text-white px-4 py-2 rounded-lg hover:bg-primary-700 mb-4"
                    >
                      <span>+</span>
                      发布动态
                    </Link>
                  )}
                  {updates.length === 0 ? (
                    <p className="text-gray-500 text-center py-8">暂无项目动态</p>
                  ) : (
                    updates.map((update) => (
                      <div key={update.id} className="border-l-4 border-primary-200 pl-4">
                        <h4 className="font-semibold text-gray-900">{update.title}</h4>
                        <p className="text-sm text-gray-500 mb-2">
                          {new Date(update.createdAt).toLocaleDateString()}
                        </p>
                        <p className="text-gray-600">{update.content}</p>
                      </div>
                    ))
                  )}
                </div>
              )}

              {activeTab === 'donations' && (
                <div className="space-y-4">
                  {donations.length === 0 ? (
                    <p className="text-gray-500 text-center py-8">暂无捐赠记录</p>
                  ) : (
                    donations.map((donation) => (
                      <div key={donation.id} className="flex items-center justify-between p-4 bg-gray-50 rounded-lg">
                        <div className="flex items-center gap-3">
                          <div className="w-10 h-10 bg-primary-100 rounded-full flex items-center justify-center">
                            <span className="text-primary-600 font-medium">
                              {(donation.donorName || '爱心人士').charAt(0)}
                            </span>
                          </div>
                          <div>
                            <div className="font-medium text-gray-900">{donation.donorName || '爱心人士'}</div>
                            <div className="text-sm text-gray-500">
                              {new Date(donation.createdAt).toLocaleString()}
                            </div>
                          </div>
                        </div>
                        <div className="text-right">
                          <div className="text-primary-600 font-semibold">¥{donation.amount.toLocaleString()}</div>
                          {donation.message && (
                            <div className="text-sm text-gray-500">{donation.message}</div>
                          )}
                        </div>
                      </div>
                    ))
                  )}
                </div>
              )}

              {activeTab === 'funds' && (
                <div className="space-y-8">
                  {isOwnerOrg && (
                    <div>
                      <h3 className="text-xl font-semibold mb-3">组织资金管理</h3>
                      <OrgFundManager
                        projectId={project.id}
                        settled={!!project.settledAt}
                        onChanged={() => {
                          loadProject();
                          setFunds(null);
                          loadFunds();
                        }}
                      />
                    </div>
                  )}
                  <div>
                    <h3 className="text-xl font-semibold mb-3">资金拨付与用途公示</h3>
                    {!funds ? (
                      <p className="text-gray-500">加载中...</p>
                    ) : (
                      <FundPanel summary={funds.summary} disbursements={funds.orders} vouchers={funds.vouchers} />
                    )}
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>

        <div className="col-span-1">
          <div className="bg-white rounded-2xl shadow-sm p-6 sticky top-24">
            <div className="mb-6">
              <div className="flex justify-between text-sm mb-2">
                <span className="text-gray-500">已筹金额</span>
                <span className="text-primary-600 font-semibold">{project.progress}%</span>
              </div>
              <div className="w-full bg-gray-200 rounded-full h-3 mb-3">
                <div
                  className="bg-primary-600 h-3 rounded-full"
                  style={{ width: `${project.progress}%` }}
                />
              </div>
              <div className="flex justify-between">
                <div>
                  <div className="text-2xl font-bold text-gray-900">¥{project.currentAmount.toLocaleString()}</div>
                  <div className="text-sm text-gray-500">已筹</div>
                </div>
                <div className="text-right">
                  <div className="text-2xl font-bold text-gray-400">¥{project.targetAmount.toLocaleString()}</div>
                  <div className="text-sm text-gray-500">目标</div>
                </div>
              </div>
            </div>

            <button
              onClick={handleDonateClick}
              disabled={project.status === 'completed'}
              className="w-full bg-primary-600 text-white py-4 rounded-xl font-semibold text-lg hover:bg-primary-700 disabled:bg-gray-300 disabled:cursor-not-allowed mb-4"
            >
              {project.status === 'completed' ? '项目已完成' : '立即捐赠'}
            </button>

            <div className="text-center text-sm text-gray-500">
              已有 {donations.length} 人参与捐赠
            </div>
          </div>
        </div>
      </div>

      <DonationModal
        project={project}
        isOpen={showDonationModal}
        onClose={() => setShowDonationModal(false)}
        onSuccess={loadProject}
      />
    </div>
  );
};

export default ProjectDetail;
