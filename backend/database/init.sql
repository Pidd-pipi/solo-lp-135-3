-- 公益捐赠追踪平台 数据库初始化脚本 (MySQL 8.0)
CREATE DATABASE IF NOT EXISTS givetrack DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE givetrack;

CREATE TABLE IF NOT EXISTS users (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  username VARCHAR(64) NOT NULL UNIQUE,
  email VARCHAR(128) NOT NULL UNIQUE,
  password_hash VARCHAR(255) NOT NULL,
  role VARCHAR(20) NOT NULL,
  real_name VARCHAR(64) DEFAULT '',
  avatar VARCHAR(255) DEFAULT '',
  phone VARCHAR(32) DEFAULT '',
  total_donation DECIMAL(14,2) DEFAULT 0,
  service_hours DECIMAL(10,2) DEFAULT 0,
  created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  INDEX idx_user_role (role)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS organizations (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL UNIQUE,
  name VARCHAR(128) NOT NULL,
  description TEXT,
  license_number VARCHAR(64) DEFAULT '',
  contact_person VARCHAR(64) DEFAULT '',
  contact_phone VARCHAR(32) DEFAULT '',
  address VARCHAR(255) DEFAULT '',
  status VARCHAR(20) DEFAULT 'pending',
  created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS projects (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  organization_id BIGINT UNSIGNED NOT NULL,
  title VARCHAR(200) NOT NULL,
  description TEXT,
  category VARCHAR(32) NOT NULL,
  target_amount DECIMAL(14,2) NOT NULL,
  current_amount DECIMAL(14,2) DEFAULT 0,
  execution_plan TEXT,
  cover_image VARCHAR(255) DEFAULT '',
  status VARCHAR(20) DEFAULT 'pending',
  start_date DATE NULL,
  end_date DATE NULL,
  settled_at DATETIME(3) NULL,
  created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  INDEX idx_project_org (organization_id),
  INDEX idx_project_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS project_updates (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  project_id BIGINT UNSIGNED NOT NULL,
  title VARCHAR(200) NOT NULL,
  content TEXT,
  images TEXT,
  created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3),
  INDEX idx_update_project (project_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS donations (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  project_id BIGINT UNSIGNED NOT NULL,
  amount DECIMAL(14,2) NOT NULL,
  payment_method VARCHAR(20) DEFAULT '',
  payment_status VARCHAR(20) DEFAULT 'success',
  transaction_id VARCHAR(64) DEFAULT '',
  certificate_no VARCHAR(64) DEFAULT '',
  is_anonymous TINYINT(1) DEFAULT 0,
  message VARCHAR(255) DEFAULT '',
  created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3),
  INDEX idx_donation_user (user_id),
  INDEX idx_donation_project (project_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS admin_reviews (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  project_id BIGINT UNSIGNED DEFAULT 0,
  organization_id BIGINT UNSIGNED DEFAULT 0,
  reviewer_id BIGINT UNSIGNED DEFAULT 0,
  status VARCHAR(20) DEFAULT '',
  comment VARCHAR(255) DEFAULT '',
  created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS volunteer_services (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  project_id BIGINT UNSIGNED DEFAULT 0,
  hours DECIMAL(8,2) NOT NULL,
  service_at DATETIME(3) NULL,
  created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3),
  INDEX idx_vs_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 用款申请：待审核(pending)与已通过(approved)合计占用已筹资金额度
CREATE TABLE IF NOT EXISTS disbursement_applications (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  project_id BIGINT UNSIGNED NOT NULL,
  org_id BIGINT UNSIGNED NOT NULL,
  applicant_id BIGINT UNSIGNED NOT NULL,
  amount DECIMAL(14,2) NOT NULL,
  purpose VARCHAR(500) NOT NULL,
  batch_no VARCHAR(64) DEFAULT '',
  status VARCHAR(20) NOT NULL DEFAULT 'pending',
  reviewer_id BIGINT UNSIGNED DEFAULT 0,
  review_comment VARCHAR(500) DEFAULT '',
  reviewed_at DATETIME(3) NULL,
  created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  INDEX idx_fa_project (project_id),
  INDEX idx_fa_org (org_id),
  INDEX idx_fa_applicant (applicant_id),
  INDEX idx_fa_status (status),
  INDEX idx_fa_reviewed (reviewed_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 拨付单：审核通过后唯一生成（application_id 唯一）
CREATE TABLE IF NOT EXISTS disbursement_orders (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  order_no VARCHAR(64) NOT NULL UNIQUE,
  application_id BIGINT UNSIGNED NOT NULL UNIQUE,
  project_id BIGINT UNSIGNED NOT NULL,
  org_id BIGINT UNSIGNED NOT NULL,
  amount DECIMAL(14,2) NOT NULL,
  purpose VARCHAR(500) DEFAULT '',
  status VARCHAR(20) NOT NULL DEFAULT 'pending',
  paid_at DATETIME(3) NULL,
  created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  INDEX idx_do_project (project_id),
  INDEX idx_do_org (org_id),
  INDEX idx_do_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 支出凭证：组织回填实际用途，挂靠已审核拨付单
CREATE TABLE IF NOT EXISTS expense_vouchers (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  order_id BIGINT UNSIGNED NOT NULL,
  application_id BIGINT UNSIGNED NOT NULL,
  project_id BIGINT UNSIGNED NOT NULL,
  amount DECIMAL(14,2) NOT NULL,
  category VARCHAR(64) NOT NULL,
  usage VARCHAR(500) NOT NULL,
  voucher_no VARCHAR(64) DEFAULT '',
  invoice_no VARCHAR(64) DEFAULT '',
  attachment_url VARCHAR(255) DEFAULT '',
  progress_note TEXT,
  spent_at DATETIME(3) NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'pending',
  checker_id BIGINT UNSIGNED DEFAULT 0,
  checked_at DATETIME(3) NULL,
  created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  INDEX idx_ev_order (order_id),
  INDEX idx_ev_application (application_id),
  INDEX idx_ev_project (project_id),
  INDEX idx_ev_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
