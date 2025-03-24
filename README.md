# Event-Driven Secrets Manager (EDSM)
⚡ Keep Kubernetes Secrets Always Up-to-Date with Real-Time Cloud Events

<div style="border-left: 4px solid #007acc; padding-left: 15px; font-size: 16px; max-width: 100%; word-wrap: break-word;">

Event-Driven Secrets Manager (EDSM) ensures that applications in Kubernetes always use the latest credentials and API keys from cloud providers—without unnecessary polling, API costs, or manual intervention.

</div>


## 📌 Key Features
- **Event-Driven Architecture** → Reacts to real-time updates from AWS Secrets Manager. (GCP & Azure planned)
- **Automatic Secret Sync** → Updates Kubernetes Secrets only when a change occurs.
- **Deployment Rollout Automation** → Triggers rolling updates for deployments using updated secrets.
- **Drift Detection & Self-Healing** → Prevents manual edits from causing configuration inconsistencies.
- **Scalable & Cost-Efficient** → Reduces API calls to cloud providers, lowering costs and improving performance.


## 🛠 How It Works
- 1️⃣ Cloud Secret Update → A secret is modified in AWS Secrets Manager. (GCP & Azure planned)
- 2️⃣ Cloud Event Notification → AWS EventBridge → SQS sends an event. (Similar services for GCP & Azure TBD)
- 3️⃣ EDSM Listens for Updates → The controller detects the change and fetches the new secret.
- 4️⃣ Kubernetes Secret Sync → The updated secret is stored securely in Kubernetes.
- 5️⃣ Deployment Rollout → If a deployment uses this secret, it is automatically restarted to apply the change.
- 6️⃣ Drift Detection → If someone manually edits the Kubernetes Secret, it is reverted to match the cloud provider version.


## 🛠 Prerequisites
**✅ AWS (Fully Implemented & Tested)**
- **AWS Secrets Manager** → Stores and manages secrets.
- **AWS CloudTrail** → Captures secret updates.
- **AWS EventBridge** → Triggers an event when a secret is updated.
- **AWS SQS** → Pushes events to the EDSM controller.
- **Kubernetes Cluster** → Stores the secret and rolls out updates.

**🔄 GCP (Planned, Not Implemented Yet)**
- **GCP Secret Manager** → Stores and manages secrets.
- **GCP Audit Logs (CloudTrail Equivalent)** → Captures secret updates.
- **GCP Eventarc (EventBridge Equivalent)** → Triggers an event when a secret is updated.
- **GCP Pub/Sub (SQS Equivalent)** → Pushes events to the EDSM controller.

**🚧 Azure (Future Implementation)**
- **Azure Key Vault** → Stores secrets.
- **Azure Monitor Logs** (CloudTrail Equivalent) → Tracks secret updates.
- **Azure Event Grid** (EventBridge Equivalent) → Triggers an event when a secret is updated.
- **Azure Service Bus** (SQS Equivalent) → Pushes events to the EDSM controller.

## 🎯 Why Use EDSM?
- **Prevent Service Downtime** → Ensure applications always have the correct credentials.
- **Reduce API Costs** → Avoid unnecessary cloud provider API calls.
- **Improve Security** → Automatically sync secrets, eliminating the risk of outdated or compromised credentials.
- **Simplify Operations** → No manual restarts, no need to monitor secret updates manually.

## 🚧 Current Limitations
- ❌ **No GCP or Azure support yet** → Currently, only AWS Secrets Manager is supported.
- ⚠️ **Not tested with multiple replicas** → Running multiple controller instances may cause event duplication.
- 🔍 **Only JSON-formatted secrets are supported** → No support for raw text secrets at this time.
- 🛑 **No admission controller yet** → Manual edits to target secrets are reverted but not blocked.

## 📌 Future Enhancements
- **GCP & Azure Support** → Expanding integrations beyond AWS.
- **Admission Controller for Secret Validation** → Prevent unauthorized manual edits to Kubernetes Secrets.
- **Metrics & Monitoring** → Track secret updates, rollouts, and API interactions.
- **Fine-Grained Key Selection** → Support for fetching specific keys inside a secret (JSON formatted).

## 📖 Getting Started

### 🚀 Deployment
#### Step 1: Set Up Cloud Provider Prerequisites
Before deploying EDSM, you need to configure your cloud provider to send secret update events.
- **✅ AWS (Fully Implemented)**
  - **Terraform Setup:**
  ```shell
  cd examples/terraform/aws-prerequisites  # AWS (currently available)
  terraform init
  terraform apply
  ```
  - Retrieve the required values:
  ```sh
  terraform output edsm_role_arn  # IAM Role ARN for EDSM
  terraform output sqs_queue_url  # AWS SQS Queue URL
  ```

- **✅ GCP (Planned)**
  - Similar setup with **Eventarc → Pub/Sub** (TBD).

- **✅ Azure (Future Implementation)**
  - Similar setup with **Event Grid → Service Bus** (TBD).

#### Step 2:  Deploy EDSM on Kubernetes
Use Helm to deploy the controller:
- Configure values.yaml before deployment:
    - Uncomment and set the required values:
      ```yaml
       extraEnv:
         - name: SQS_QUEUE_URL
           value: "https://sqs.eu-west-1.amazonaws.com/123456798123/secrets-manager-events-queue"
       serviceAccount:
         annotations:
           eks.amazonaws.com/role-arn: "arn:aws:iam::123456789123:role/my-cluster-edsm-service-account"
      ```
      → For full configuration, see [values.yaml](charts/k8s-event-driven-secrets/values.yaml).
```sh
helm upgrade --install edsm charts/k8s-event-driven-secrets --namespace kube-system
```

## 📖 Usage Guide
### 1️⃣ Define Event-Driven Secrets in Kubernetes
📌 Use an EventDrivenSecret CRD to **sync secrets from cloud providers to Kubernetes automatically**.
```yaml
apiVersion: secrets.edsm.io/v1alpha1
kind: EventDrivenSecret
metadata:
  name: my-secret-1
  namespace: default
spec:
  cloudProvider: aws          # Cloud provider (aws, GCP/Azure planned)
  cloudProviderOptions:
    region: eu-west-3           # Cloud Provider Region where the secret exists
  secretPath: staging/common-secrets # Path to the cloud secret
  targetSecretName: my-secret-1 # The name of the Kubernetes Secret to create/update
```
### ✅ How it Works:
- When staging/common-secrets is updated in AWS Secrets Manager, **EDSM automatically updates** my-secret-1 in Kubernetes.
- Any **deployment using my-secret-1 will be restarted** to pick up the changes. 
  - More details on how deployments detect secret changes → [Deployment Annotations: Ensuring Services Pick Up New Secrets.](#deployment-annotations-ensuring-services-pick-up-new-secrets)

### Deployment Annotations: Ensuring Services Pick Up New Secrets
#### Why Do Deployments Need Annotations?
By default, Kubernetes does **not automatically restart** a pod when a secret is updated. To ensure services reload updated credentials, **EDSM requires deployments to have a special annotation.**
**📌 Example Deployment with** eventdrivensecretsmanaged **Annotation**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  namespace: default
  annotations:
    eventdrivensecretsmanaged: '["my-secret-1", "db-credentials"]'  # 👈 List of secrets to watch
spec:
  replicas: 2
  selector:
    matchLabels:
      app: my-app
  template:
    metadata:
      labels:
        app: my-app
    spec:
      containers:
      - name: my-app
        image: my-app:latest
        env:
        - name: DB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: db-credentials   # 👈 Automatically updated by EDSM
              key: DB_PASSWORD
```
#### ✅ How it Works: 
- 1️⃣ EDSM updates db-credentials when the cloud secret changes.
- 2️⃣ The **controller detects that** db-credentials **is referenced** by this deployment.
- 3️⃣ A **rolling update is triggered**, so all pods restart and use the new secret.
- **🔍 Key Takeaways:**
  - The annotation must contain a JSON array of secrets that the deployment depends on.
  - If this annotation is missing, the deployment will NOT automatically restart when secrets change.
  - This ensures applications always use the latest credentials with minimal downtime.

## 🛠 Development Guide
### Prerequisites
- **Go** version v1.23.0+
- **Docker** version 17.03+.
- **Kubectl** version v1.11.3+.
- **Kind** (for local Kubernetes testing)
- **Kubebuilder** (for code generation)

### 🚧 Essential Kubebuilder Commands
These **Kubebuilder** commands **auto-generate** and update project files, so **never manually modify** generated files:
  - **Generate CRDs** and manifests (run this if api/v1alpha1/eventdrivensecret_types.go changes):
     ```sh
     make manifests
     ```
  - **Rebuild Go source and update dependencies:**
     ```sh
     make generate
     ```
  - **Run locally with Kind (for local Kubernetes cluster testing):**
       ```sh
      make kind
      ```
📌 For more advanced customization, refer to the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html).


## 🤝 Contributing to EDSM
We welcome contributions that help improve **Event-Driven Secrets Manager (EDSM)!** 🎉
To maintain stability and quality, please follow these guidelines before contributing.
### 💡 How to Contribute
#### 1️⃣ Fork the Repository
Since new branches cannot be created directly in this repo, contributors must fork the repository:
  - Click **Fork** on the top right of the repository page in GitHub.
  - Clone your forked repository to your local machine:
    ```shell
    git clone https://github.com/kkuyumjyan/k8s-event-driven-secrets.git
    cd k8s-event-driven-secrets
    ```
#### 2️⃣ Create a New Branch
- Always create a new branch for your changes:
  ```shell
  git checkout -b feature/my-new-feature
  ```
- Follow branch naming conventions:
  - feature/<name> → For new features.
  - bugfix/<name> → For bug fixes.
  - docs/<name> → For documentation improvements.

#### Make Changes and Test Locally
- Implement your feature, bugfix, or documentation change.
- Ensure your code follows best practices and formatting:
  ```shell
  make manifests  # Regenerate CRDs if necessary
  make generate   # Update Go files if needed
  make fmt        # Format Go code
  ```

#### Commit and Push Your Changes
- Use **clear, descriptive commit messages:** 
  ```shell
  git add .
  git commit -m "feat: Add new feature to sync secrets"
  git push origin feature/my-new-feature
  ```

#### 5️⃣ Open a Pull Request (PR)
- Go to the [**original repository**](https://github.com/kkuyumjyan/k8s-eventdriven-secrets) on GitHub.
- Click **New Pull Request.**
- Select your fork and branch as the source.
- Provide a **clear description** of your changes in the PR.
- Wait for maintainers to review and approve.

#### 🔄 Review & Approval Process
- A maintainer will **review your PR**, provide feedback, or approve it.
- If changes are requested, **update your branch** and **push** the changes:
  ```shell
  git add .
  git commit -m "fix: Address review feedback"
  git push origin feature/my-new-feature
  ```

#### ✅ Contribution Guidelines
- **Follow the existing project structure.**
- **Keep changes minimal and focused on a single improvement.**
- **Write clear commit messages** (No “Fix stuff” commits).
- **Ensure tests pass before submitting a PR.**
- **Respect maintainers’ feedback and requests.**

#### ❌ What Contributions Will Be Rejected?
- **Large PRs** that combine multiple features or fixes.
- **Breaking changes** without proper discussion.
- **Poorly documented PRs** that lack descriptions.
- **Modifications that complicate maintenance** without clear benefits.
- **Feature requests outside the defined scope** (e.g., unrelated sync mechanisms).

#### 📢 Need Help?
If you're unsure about anything, open an issue in GitHub to discuss your idea before starting a PR.
We appreciate your contributions! 🎉

## License

Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

