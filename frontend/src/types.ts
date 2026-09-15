export type UserRole = 'user' | 'org' | 'admin';

export interface User {
  id: string;
  username: string;
  email: string;
  role: UserRole;
  avatar?: string;
  phone?: string;
  realName?: string;
  totalDonation: number;
  serviceHours: number;
  createdAt: string;
}

export type ProjectCategory = 'education' | 'elderly' | 'medical' | 'disaster' | 'environment' | 'other';
export type ProjectStatus = 'pending' | 'approved' | 'rejected' | 'completed';

export interface Project {
  id: string;
  organizationId: string;
  title: string;
  description?: string;
  category: ProjectCategory;
  targetAmount: number;
  currentAmount: number;
  executionPlan?: string;
  coverImage?: string;
  status: ProjectStatus;
  startDate?: string;
  endDate?: string;
  settledAt?: string;
  createdAt: string;
  progress?: number;
  organization?: Organization;
}

export type OrgStatus = 'pending' | 'approved' | 'rejected';

export interface Organization {
  id: string;
  userId: string;
  name: string;
  description?: string;
  licenseNumber?: string;
  contactPerson?: string;
  contactPhone?: string;
  address?: string;
  status: OrgStatus;
  createdAt: string;
}

export type PaymentMethod = 'wechat' | 'alipay' | 'bank';
export type PaymentStatus = 'pending' | 'success' | 'failed';

export interface Donation {
  id: string;
  userId: string;
  projectId: string;
  amount: number;
  paymentMethod: PaymentMethod;
  paymentStatus: PaymentStatus;
  transactionId?: string;
  certificateNo?: string;
  certificateUrl?: string;
  isAnonymous: boolean;
  message?: string;
  createdAt: string;
  donorName?: string;
  user?: User;
  project?: Project;
}

export interface ProjectUpdate {
  id: string;
  projectId: string;
  title: string;
  content?: string;
  images?: string;
  createdAt: string;
}

export interface RankingItem {
  id: string;
  userId: string;
  username: string;
  avatar?: string;
  realName?: string;
  totalDonation: number;
  serviceHours: number;
  rank: number;
}

// ===== 资金拨付与用途凭证 =====

export type FundApplicationStatus = 'pending' | 'approved' | 'rejected';
export type DisbursementStatus = 'pending' | 'paid';
export type VoucherStatus = 'pending' | 'checked';

export interface FundApplication {
  id: string;
  projectId: string;
  orgId: string;
  applicantId: string;
  amount: number;
  purpose: string;
  batchNo: string;
  status: FundApplicationStatus;
  reviewerId?: number;
  reviewComment?: string;
  reviewedAt?: string;
  createdAt: string;
  updatedAt?: string;
  project?: Project;
  order?: DisbursementOrder;
}

export interface DisbursementOrder {
  id: string;
  orderNo: string;
  applicationId: string;
  projectId: string;
  orgId: string;
  amount: number;
  purpose: string;
  status: DisbursementStatus;
  paidAt?: string;
  createdAt: string;
  vouchers?: ExpenseVoucher[];
}

export interface ExpenseVoucher {
  id: string;
  orderId: string;
  applicationId: string;
  projectId: string;
  amount: number;
  category: string;
  usage: string;
  voucherNo: string;
  invoiceNo?: string;
  attachmentUrl?: string;
  progressNote?: string;
  spentAt: string;
  status: VoucherStatus;
  createdAt: string;
}

export interface FundSummary {
  projectId: number;
  projectTitle: string;
  raisedAmount: number;
  occupiedAmount: number;
  disbursedAmount: number;
  usedAmount: number;
  pendingAmount: number;
  remainingAmount: number;
}
