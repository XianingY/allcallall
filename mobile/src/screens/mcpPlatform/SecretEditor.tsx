import React from "react";
import { Text, View } from "react-native";

import type { SecretDraft } from "../mcpPlatformUtils";
import { ActionButton, Field } from "./primitives";
import { styles } from "./styles";

interface SecretEditorProps {
  secretDrafts: SecretDraft[];
  setSecretDrafts: React.Dispatch<React.SetStateAction<SecretDraft[]>>;
  secretErrors: Record<string, string>;
  canManageSelectedInstallation: boolean;
  onUpdateSecretRow: (index: number, patch: Partial<SecretDraft>) => void;
  busyKey: string;
  onSaveSecrets: () => void;
}

/** 安装详情里的 Secret 编辑区（#24 拆分时从 InstallationsPanel 再抽一层）。 */
export const SecretEditor: React.FC<SecretEditorProps> = ({
  secretDrafts,
  setSecretDrafts,
  secretErrors,
  canManageSelectedInstallation,
  onUpdateSecretRow,
  busyKey,
  onSaveSecrets
}) => (
  <>
                  <View style={styles.divider} />
                  <Text style={styles.formTitle}>Secret 字段</Text>
                  <Text style={styles.sectionMeta}>
                    已保存的值不会回显
                  </Text>
                  {secretDrafts.map((secret, index) => (
                    <View key={index} style={styles.secretRow}>
                      <View style={styles.secretField}>
                        <Field
                          label="名称"
                          value={secret.key}
                          editable={canManageSelectedInstallation}
                          autoCapitalize="characters"
                          error={secretErrors[String(index)]}
                          onChangeText={(key) => onUpdateSecretRow(index, { key })}
                          placeholder="API_TOKEN"
                        />
                      </View>
                      <View style={styles.secretField}>
                        <Field
                          label="值"
                          value={secret.value}
                          editable={canManageSelectedInstallation}
                          secureTextEntry
                          autoCapitalize="none"
                          onChangeText={(value) => onUpdateSecretRow(index, { value })}
                          placeholder="Secret value"
                        />
                      </View>
                      {secretDrafts.length > 1 ? (
                        <ActionButton
                          label="移除"
                          compact
                          danger
                          disabled={!canManageSelectedInstallation}
                          onPress={() =>
                            setSecretDrafts((current) =>
                              current.filter((_, itemIndex) => itemIndex !== index),
                            )
                          }
                        />
                      ) : null}
                    </View>
                  ))}
                  {secretErrors.form ? <Text style={styles.errorText}>{secretErrors.form}</Text> : null}
                  <View style={styles.actionRow}>
                    <ActionButton
                      label="添加字段"
                      compact
                      disabled={!canManageSelectedInstallation}
                      onPress={() =>
                        setSecretDrafts((current) => [...current, { key: "", value: "" }])
                      }
                    />
                    <ActionButton
                      label={busyKey === "save-secrets" ? "保存中..." : "安全保存"}
                      compact
                      disabled={busyKey !== "" || !canManageSelectedInstallation}
                      onPress={() => void onSaveSecrets()}
                    />
                  </View>
  </>
);
