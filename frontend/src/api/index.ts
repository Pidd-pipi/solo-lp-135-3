import axios from 'axios';

const api = axios.create({
  baseURL: '/api',
  timeout: 10000,
});

api.interceptors.request.use((config) => {
  const token = localStorage.getItem('token');
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

api.interceptors.response.use(
  (response) => {
    // 解包后端统一响应 {code, message, data}，code===0 时返回 data
    if (response.data && typeof response.data === 'object' && 'code' in response.data) {
      if (response.data.code === 0) {
        response.data = response.data.data;
      }
    }
    return response;
  },
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token');
      localStorage.removeItem('user');
      window.location.href = '/login';
    }
    return Promise.reject(error);
  }
);

export const authAPI = {
  register: (data: any) => api.post('/auth/register', data),
  login: (data: any) => api.post('/auth/login', data),
  getMe: () => api.get('/auth/me'),
  updateProfile: (data: any) => api.put('/auth/profile', data),
};

export const projectAPI = {
  getProjects: (params?: any) => api.get('/projects', { params }),
  getProject: (id: string) => api.get(`/projects/${id}`),
  createProject: (data: any) => api.post('/projects', data),
  getMyProjects: () => api.get('/projects/org/my'),
  getProjectUpdates: (projectId: string) => api.get(`/projects/${projectId}/updates`),
  createProjectUpdate: (projectId: string, data: any) => api.post(`/projects/${projectId}/updates`, data),
};

export const donationAPI = {
  createDonation: (data: any) => api.post('/donations', data),
  getMyDonations: (params?: any) => api.get('/donations/my', { params }),
  getCertificate: (id: string) => api.get(`/donations/${id}/certificate`),
};

export const rankingAPI = {
  getDonationRanking: (limit?: number) => api.get('/ranking/donation', { params: { limit } }),
  getServiceRanking: (limit?: number) => api.get('/ranking/service', { params: { limit } }),
  getStats: () => api.get('/ranking/stats'),
};

export const adminAPI = {
  getPendingProjects: () => api.get('/admin/projects/pending'),
  reviewProject: (id: string, data: any) => api.post(`/admin/projects/${id}/review`, data),
  getPendingOrganizations: () => api.get('/admin/organizations/pending'),
  reviewOrganization: (id: string, data: any) => api.post(`/admin/organizations/${id}/review`, data),
};

// 资金拨付与用途凭证
export const fundAPI = {
  // 捐赠人公示：已审核拨付单、支出凭证与资金汇总
  getPublicFunds: (projectId: string) => api.get(`/projects/${projectId}/funds`),
  // 组织：提交用款申请
  apply: (projectId: string, data: { amount: number; purpose: string; batchNo?: string }) =>
    api.post(`/projects/${projectId}/disbursements`, data),
  // 组织：结项
  settle: (projectId: string) => api.post(`/projects/${projectId}/settle`),
  // 组织：我的用款申请
  getMyApplications: (params?: any) => api.get('/disbursements/org/my', { params }),
  // 申请详情（含拨付单与凭证）
  getApplication: (id: string) => api.get(`/disbursements/applications/${id}`),
  // 组织：回填支出凭证
  addVoucher: (applicationId: string, data: any) =>
    api.post(`/disbursements/applications/${applicationId}/vouchers`, data),
  // 平台：待审核申请
  getPendingApplications: (params?: any) => api.get('/admin/disbursements/pending', { params }),
  // 平台：全量申请追溯
  getAllApplications: (params?: any) => api.get('/admin/disbursements/applications', { params }),
  // 平台：审核（通过生成唯一拨付单 / 驳回释放额度）
  reviewApplication: (id: string, data: { approve: boolean; comment?: string }) =>
    api.post(`/admin/disbursements/applications/${id}/review`, data),
  // 平台：核验支出凭证
  checkVoucher: (id: string) => api.post(`/admin/disbursements/vouchers/${id}/check`),
};

export default api;
