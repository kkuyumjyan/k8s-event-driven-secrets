data "aws_eks_cluster" "this" {
  name = var.cluster_name
}

data "tls_certificate" "eks_cluster_certificate" {
  url = data.aws_eks_cluster.this.identity[0].oidc[0].issuer
}

data "aws_iam_openid_connect_provider" "this_eks_oidc" {
  url  = data.aws_eks_cluster.this.identity.0.oidc.0.issuer
}

data "aws_iam_policy_document" "edsm_allowed_actions" {
  statement {
    sid       = ""
    effect    = "Allow"
    resources = ["*"]
    actions = [
      "sqs:*",
      "secretsmanager:DescribeSecret",
      "secretsmanager:GetSecretValue",
      "secretsmanager:ListSecrets"
    ]
  }
}

data "aws_iam_policy_document" "edsm_assume_role_policy" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]
    principals {
      type        = "Federated"
      identifiers = [data.aws_iam_openid_connect_provider.this_eks_oidc.arn]
    }
    condition {
      test     = "StringEquals"
      variable = "${replace(data.aws_eks_cluster.this.identity.0.oidc.0.issuer, "https://", "")}:sub"
      values = [
        "system:serviceaccount:kube-system:${var.edsm_service_account_name}"
      ]
    }
  }
}


resource "aws_iam_policy" "edsm_allowed_actions" {
  name        = "${var.cluster_name}-edsm-allowed-actions"
  path        = "/"
  description = "Policy for k8s-event-driven-secrets manager"
  policy      = data.aws_iam_policy_document.edsm_allowed_actions.json
}

resource "aws_iam_role" "edsm_service_account" {
  name               = "${var.cluster_name}-edsm-service-account"
  assume_role_policy = data.aws_iam_policy_document.edsm_assume_role_policy.json
}

resource "aws_iam_role_policy_attachment" "edsm_allowed_actions" {
  policy_arn = aws_iam_policy.edsm_allowed_actions.arn
  role       = aws_iam_role.edsm_service_account.name
}
