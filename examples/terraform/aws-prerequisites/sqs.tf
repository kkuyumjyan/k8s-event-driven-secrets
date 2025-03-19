# Create an SQS Queue
resource "aws_sqs_queue" "secrets_manager_events_queue" {
  name                      = "secrets-manager-events-queue"
  message_retention_seconds = 86400
}

# Allow EventBridge to send messages to SQS
resource "aws_sqs_queue_policy" "secrets_manager_queue_policy" {
  queue_url = aws_sqs_queue.secrets_manager_events_queue.id

  policy = <<EOF
{
  "Version": "2012-10-17",
  "Id": "EventBridgeSQSPolicy",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Service": "events.amazonaws.com"
      },
      "Action": "sqs:SendMessage",
      "Resource": "${aws_sqs_queue.secrets_manager_events_queue.arn}",
      "Condition": {
        "ArnEquals": {
          "aws:SourceArn": "${aws_cloudwatch_event_rule.secrets_manager_rule.arn}"
        }
      }
    }
  ]
}
EOF
}
