import { useCallback, useState } from "react";
import { Alert } from "react-native";

import { createAbuseReport } from "../../api/commercial";
import type { User } from "../../api/users";
import type { ReportCategory } from "./types";

/**
 * 举报流程的状态与动作（#24 拆分时从 ContactsScreen 抽出）。
 * 只依赖 token，因此是自包含的——不含任何与联系人列表相关的副作用。
 */
export const useReportFlow = (token: string | null) => {
  const [reportTarget, setReportTarget] = useState<User | null>(null);
  const [reportCategory, setReportCategory] =
    useState<ReportCategory>("harassment");
  const [reportDetails, setReportDetails] = useState("");
  const [submittingReport, setSubmittingReport] = useState(false);

  const openReportModal = useCallback((contact: User) => {
    setReportTarget(contact);
    setReportCategory("harassment");
    setReportDetails("");
  }, []);

  const closeReportModal = useCallback(() => {
    setReportTarget(null);
    setReportDetails("");
  }, []);

  const submitReport = useCallback(async () => {
    if (!token || !reportTarget) {
      return;
    }
    try {
      setSubmittingReport(true);
      await createAbuseReport(token, {
        reported_user_id: reportTarget.id,
        category: reportCategory,
        details:
          reportDetails.trim() ||
          `Reported from contacts list for ${reportTarget.email}`
      });
      setReportTarget(null);
      setReportCategory("harassment");
      setReportDetails("");
      Alert.alert("举报已提交", "支持团队会根据记录进行处理。");
    } catch (error) {
      console.error("[ContactsScreen] Failed to report user:", error);
      Alert.alert("提交失败", "当前无法提交举报。");
    } finally {
      setSubmittingReport(false);
    }
  }, [reportCategory, reportDetails, reportTarget, token]);

  return {
    reportTarget,
    reportCategory,
    setReportCategory,
    reportDetails,
    setReportDetails,
    submittingReport,
    openReportModal,
    closeReportModal,
    submitReport
  };
};
