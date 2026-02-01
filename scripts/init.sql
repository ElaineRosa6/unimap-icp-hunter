-- UniMap + ICP-Hunter 数据库初始化脚本

-- 创建数据库（如果不存在）
CREATE DATABASE IF NOT EXISTS `unimap_icp_hunter`
CHARACTER SET utf8mb4
COLLATE utf8mb4_unicode_ci;

USE `unimap_icp_hunter`;

-- 资产表
CREATE TABLE IF NOT EXISTS `assets` (
  `id` BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `ip` VARCHAR(45) NOT NULL,
  `port` INT NOT NULL,
  `protocol` VARCHAR(20) NOT NULL,
  `host` VARCHAR(255),
  `url` VARCHAR(512) NOT NULL,
  `title` VARCHAR(512),
  `body_snippet` TEXT,
  `server` VARCHAR(255),
  `headers` JSON,
  `status_code` INT,
  `country_code` VARCHAR(10),
  `region` VARCHAR(50),
  `city` VARCHAR(50),
  `asn` VARCHAR(20),
  `org` VARCHAR(255),
  `isp` VARCHAR(100),
  `sources` JSON,
  `extra` JSON,
  `first_seen_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  `last_seen_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `latest_check_id` BIGINT,
  `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX `idx_ip_port` (`ip`, `port`),
  INDEX `idx_url` (`url`),
  INDEX `idx_last_seen` (`last_seen_at`),
  INDEX `idx_latest_check` (`latest_check_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ICP检测记录表
CREATE TABLE IF NOT EXISTS `icp_checks` (
  `id` BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `asset_id` BIGINT UNSIGNED NOT NULL,
  `check_time` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  `url` VARCHAR(512) NOT NULL,
  `http_status_code` INT,
  `title` VARCHAR(512),
  `icp_code` VARCHAR(100),
  `is_registered` TINYINT NOT NULL DEFAULT 0,
  `match_method` VARCHAR(20),
  `html_hash` VARCHAR(64),
  `screenshot_path` VARCHAR(512),
  `error_message` TEXT,
  `tags` JSON,
  `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX `idx_asset_check` (`asset_id`, `check_time`),
  INDEX `idx_check_time` (`check_time`),
  INDEX `idx_is_registered` (`is_registered`),
  INDEX `idx_html_hash` (`html_hash`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 扫描策略表
CREATE TABLE IF NOT EXISTS `scan_policies` (
  `id` BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `name` VARCHAR(100) NOT NULL UNIQUE,
  `uql` TEXT NOT NULL,
  `engines` JSON,
  `page_size` INT DEFAULT 100,
  `max_records` INT DEFAULT 5000,
  `ports` JSON,
  `enabled` TINYINT DEFAULT 1,
  `description` TEXT,
  `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 扫描任务表
CREATE TABLE IF NOT EXISTS `scan_tasks` (
  `id` BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `policy_id` BIGINT UNSIGNED NOT NULL,
  `status` VARCHAR(20) NOT NULL,
  `start_time` TIMESTAMP NULL,
  `end_time` TIMESTAMP NULL,
  `total_candidates` INT DEFAULT 0,
  `total_probed` INT DEFAULT 0,
  `total_unregistered` INT DEFAULT 0,
  `stats_summary` JSON,
  `error_message` TEXT,
  `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX `idx_policy_id` (`policy_id`),
  INDEX `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 白名单表
CREATE TABLE IF NOT EXISTS `whitelist` (
  `id` BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `type` VARCHAR(20) NOT NULL,
  `value` VARCHAR(512) NOT NULL,
  `reason` TEXT,
  `creator` VARCHAR(100),
  `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX `idx_type_value` (`type`, `value`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 插入默认策略
INSERT INTO `scan_policies` (`name`, `uql`, `engines`, `page_size`, `max_records`, `ports`, `enabled`, `description`) VALUES
('http_80', 'country="CN" && port="80" && protocol="http"', '["fofa", "hunter"]', 100, 5000, '[80]', 1, 'HTTP 80端口扫描'),
('https_443', 'country="CN" && port="443" && protocol="https"', '["fofa", "hunter"]', 100, 5000, '[443]', 1, 'HTTPS 443端口扫描'),
('common_alt_ports', 'country="CN" && port IN ["8080", "8443", "8000", "9000"]', '["fofa", "hunter"]', 50, 2000, '[8080, 8443, 8000, 9000]', 1, '常用替代端口扫描');

-- 插入默认白名单（敏感域名）
INSERT INTO `whitelist` (`type`, `value`, `reason`, `creator`) VALUES
('domain', 'gov.cn', '政府域名', 'system'),
('domain', 'edu.cn', '教育域名', 'system'),
('domain', 'mil.cn', '军事域名', 'system');

-- 创建视图用于统计
CREATE OR REPLACE VIEW `v_daily_stats` AS
SELECT
    DATE(check_time) as check_date,
    COUNT(*) as total_scanned,
    SUM(CASE WHEN is_registered = 0 THEN 1 ELSE 0 END) as unregistered,
    SUM(CASE WHEN is_registered = 1 THEN 1 ELSE 0 END) as registered,
    SUM(CASE WHEN is_registered = 2 THEN 1 ELSE 0 END) as uncertain
FROM icp_checks
GROUP BY DATE(check_time);

-- 创建视图用于未备案列表
CREATE OR REPLACE VIEW `v_unregistered_assets` AS
SELECT
    a.id,
    a.ip,
    a.port,
    a.url,
    a.title,
    a.region,
    a.country_code,
    c.check_time,
    c.icp_code,
    c.screenshot_path
FROM assets a
JOIN icp_checks c ON a.id = c.asset_id
WHERE c.is_registered = 0
AND c.check_time = (
    SELECT MAX(check_time)
    FROM icp_checks
    WHERE asset_id = a.id
    AND is_registered = 0
);
