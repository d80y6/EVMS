module "eks" {
  source = "../../modules/eks"
  cluster_name       = "evms-staging"
  node_instance_type = "t3.large"
  node_group_min_size = 2
  node_group_max_size = 5
  environment        = "staging"
}

module "rds" {
  source = "../../modules/rds"
  db_name           = "evms_staging"
  allocated_storage = 100
  instance_class    = "db.t3.large"
  environment       = "staging"
}

module "s3" {
  source = "../../modules/s3"
  recordings_bucket = "evms-staging-recordings"
  backups_bucket    = "evms-staging-backups"
  environment       = "staging"
}
