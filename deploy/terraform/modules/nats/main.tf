locals {
  nats_name = "${var.environment}-nats"
}

resource "aws_security_group" "nats" {
  name        = local.nats_name
  description = "Security group for NATS JetStream"
  vpc_id      = var.vpc_id

  tags = {
    Name        = local.nats_name
    Environment = var.environment
  }
}

resource "aws_security_group_rule" "nats_client" {
  type              = "ingress"
  from_port         = 4222
  to_port           = 4222
  protocol          = "tcp"
  cidr_blocks       = var.allowed_cidr_blocks
  security_group_id = aws_security_group.nats.id
  description       = "NATS client connections"
}

resource "aws_security_group_rule" "nats_routes" {
  type              = "ingress"
  from_port         = 6222
  to_port           = 6222
  protocol          = "tcp"
  cidr_blocks       = var.allowed_cidr_blocks
  security_group_id = aws_security_group.nats.id
  description       = "NATS route connections"
}

resource "aws_security_group_rule" "nats_monitoring" {
  type              = "ingress"
  from_port         = 8222
  to_port           = 8222
  protocol          = "tcp"
  cidr_blocks       = var.allowed_cidr_blocks
  security_group_id = aws_security_group.nats.id
  description       = "NATS monitoring HTTP"
}

resource "aws_security_group_rule" "nats_egress" {
  type              = "egress"
  from_port         = 0
  to_port           = 0
  protocol          = "-1"
  cidr_blocks       = ["0.0.0.0/0"]
  security_group_id = aws_security_group.nats.id
}
