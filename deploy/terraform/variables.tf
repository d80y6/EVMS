variable "aws_region" {
  description = "AWS region"
  type        = string
  default     = "us-east-1"
}

variable "environment" {
  description = "Environment name (staging/production)"
  type        = string
}

variable "cluster_name" {
  description = "EKS cluster name"
  type        = string
}

variable "db_instance_class" {
  description = "RDS instance class"
  type        = string
  default     = "db.t3.large"
}

variable "node_instance_type" {
  description = "EKS node instance type"
  type        = string
  default     = "t3.large"
}

variable "node_group_min_size" {
  description = "Minimum node group size"
  type        = number
  default     = 3
}

variable "node_group_max_size" {
  description = "Maximum node group size"
  type        = number
  default     = 10
}

variable "recordings_bucket_name" {
  description = "S3 bucket name for recordings"
  type        = string
}

variable "backups_bucket_name" {
  description = "S3 bucket name for backups"
  type        = string
}
