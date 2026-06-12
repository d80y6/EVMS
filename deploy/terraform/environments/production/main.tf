module "eks" {
  source = "../../modules/eks"
  cluster_name        = "evms-production"
  kubernetes_version  = "1.28"
  node_instance_type  = "t3.large"
  node_group_min_size = 3
  node_group_max_size = 10
  node_group_desired_size = 5
  environment         = "production"
}

module "rds" {
  source = "../../modules/rds"
  db_name             = "evms_production"
  allocated_storage   = 200
  instance_class      = "db.r5.large"
  backup_retention_days = 35
  environment         = "production"
}

module "nats" {
  source = "../../modules/nats"
  environment = "production"
}

module "s3" {
  source = "../../modules/s3"
  recordings_bucket = "evms-production-recordings"
  backups_bucket    = "evms-production-backups"
  environment       = "production"
}
