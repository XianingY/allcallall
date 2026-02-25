# Work Plan: Backend Deployment to Cloud Server

## Context
The user has a cloud server with a public IP and wants to deploy the Go backend service. The project is a monorepo with a Go backend and a React Native mobile app.

### Deployment Strategy
We will use the existing `scripts/deployment/deploy-cloud.sh` script with manual adjustments to ensure it works correctly for the user's repository and configuration.

---

## Work Objectives

### Core Objective
Successfully deploy the AllCallAll backend service using Docker Compose and Nginx on a cloud server.

### Concrete Deliverables
- [ ] Updated `scripts/deployment/deploy-cloud.sh` with correct repository URL and defaults.
- [ ] Configured `.env` file on the server with secure secrets.
- [ ] Running Docker containers for Backend, MySQL, and Redis.
- [ ] Configured Nginx as a reverse proxy with SSL (optional but recommended).
- [ ] Configured UFW firewall for security.

---

## Verification Strategy

### Manual QA Procedure
- [ ] Run `curl http://<SERVER_IP>/health` (or with domain) and expect `{"status": "ok"}`.
- [ ] Check container status: `docker ps`.
- [ ] Check logs: `docker-compose logs -f backend`.
- [ ] Verify WebRTC configuration endpoint: `curl http://<SERVER_IP>/api/v1/webrtc/config`.

---

## Task Flow
1. **Script Preparation**: Update placeholders in the deployment script.
2. **Server Execution**: Run the script on the cloud server.
3. **Environment Tuning**: Manually edit the generated `.env` to add SMTP secrets.
4. **Service Startup**: Launch containers via Docker Compose.
5. **Final Verification**: Confirm service availability via public IP/domain.

---

## TODOs

### Phase 1: Local Script Preparation
- [ ] 1. Update `scripts/deployment/deploy-cloud.sh`
  - **Task**: Replace the placeholder GitHub URL with the actual repository URL.
  - **Task**: Update hardcoded email addresses to match the user's intended SMTP configuration (or make them configurable).
  - **Reference**: `scripts/deployment/deploy-cloud.sh:82`, `133`, `135`.
  - **Parallelizable**: NO

### Phase 2: Server Initialization
- [ ] 2. Execute Deployment Script on Server
  - **Task**: SSH into the server and run the script:
    ```bash
    bash scripts/deployment/deploy-cloud.sh <SERVER_IP> [DOMAIN_NAME]
    ```
  - **Manual Verification**: Check if Docker and Docker Compose are installed (`docker --version`).
  - **Parallelizable**: NO

### Phase 3: Post-Script Configuration
- [ ] 3. Finalize `.env` and `config.yaml`
  - **Task**: SSH into the server, navigate to `$WORK_DIR`.
  - **Task**: Edit `.env` to add the real `MAIL_PASSWORD` (SMTP auth code).
  - **Task**: Verify `backend/configs/config.yaml` has the correct `ice_servers` if using a TURN server.
  - **Parallelizable**: NO

### Phase 4: Service Launch & Security
- [ ] 4. Start Services and Configure SSL
  - **Task**: Run `docker-compose -f infra/docker-compose.yml up -d`.
  - **Task**: (Optional) Run `certbot --nginx` if a domain was provided.
  - **Task**: Ensure `ufw` is active and ports 22, 80, 443 are open.
  - **Parallelizable**: NO

---

## Success Criteria
- [ ] `curl http://<SERVER_IP>/health` returns success.
- [ ] Mobile app can successfully connect to the signaling service via the server IP/domain.
- [ ] MySQL and Redis are not accessible from the public internet (verified via `nmap` or similar).
