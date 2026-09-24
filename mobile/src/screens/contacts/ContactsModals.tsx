import React from "react";
import { Modal, Pressable, Text, TouchableOpacity, View } from "react-native";

import type { User } from "../../api/users";
import PrimaryButton from "../../components/PrimaryButton";
import TextField from "../../components/TextField";
import { REPORT_CATEGORIES } from "./constants";
import { styles } from "./styles";
import type { ReportCategory } from "./types";

interface AddContactModalProps {
  visible: boolean;
  onClose: () => void;
  email: string;
  onEmailChange: (value: string) => void;
  onSearch: () => void;
  searchResults: User[];
  onSelectResult: (email: string) => void;
  onAdd: () => void;
}

export const AddContactModal: React.FC<AddContactModalProps> = ({
  visible,
  onClose,
  email,
  onEmailChange,
  onSearch,
  searchResults,
  onSelectResult,
  onAdd
}) => (
  <Modal visible={visible} transparent animationType="slide" onRequestClose={onClose}>
    <View style={styles.modalBackdrop}>
      <View style={styles.modalContent}>
        <Text style={styles.modalTitle}>添加联系人 / Add Contact</Text>
        <TextField
          label="邮箱 / Email Address"
          autoCapitalize="none"
          keyboardType="email-address"
          value={email}
          onChangeText={onEmailChange}
        />
        <PrimaryButton title="搜索用户" onPress={() => void onSearch()} style={styles.searchButton} />
        {searchResults.length > 0 ? (
          <View style={styles.searchResults}>
            {searchResults.map((result) => (
              <Pressable
                key={result.id}
                style={styles.searchResultRow}
                onPress={() => onSelectResult(result.email)}
              >
                <Text style={styles.searchResultTitle}>{result.display_name || result.email}</Text>
                <Text style={styles.searchResultMeta}>{result.email}</Text>
              </Pressable>
            ))}
          </View>
        ) : null}
        <PrimaryButton title="添加 / Add" onPress={onAdd} />
        <PrimaryButton title="取消 / Cancel" onPress={onClose} style={styles.modalCancel} />
      </View>
    </View>
  </Modal>
);

interface ContactActionSheetProps {
  contact: User | null;
  onClose: () => void;
  onCall: (contact: User) => void;
  onDetail: (contact: User) => void;
  onReport: (contact: User) => void;
  onBlock: (contact: User) => void;
  onRemove: (contact: User) => void;
}

export const ContactActionSheet: React.FC<ContactActionSheetProps> = ({
  contact,
  onClose,
  onCall,
  onDetail,
  onReport,
  onBlock,
  onRemove
}) => (
  <Modal
    visible={Boolean(contact)}
    transparent
    animationType="fade"
    onRequestClose={onClose}
  >
    <Pressable style={styles.modalBackdrop} onPress={onClose}>
      <Pressable style={styles.actionSheet}>
        <Text style={styles.modalTitle}>{contact?.display_name || contact?.email}</Text>
        <TouchableOpacity style={styles.actionSheetButton} onPress={() => {
          if (contact) {
            onCall(contact);
          }
          onClose();
        }}>
          <Text style={styles.actionSheetText}>呼叫 / Call</Text>
        </TouchableOpacity>
        <TouchableOpacity style={styles.actionSheetButton} onPress={() => {
          if (contact) {
            onDetail(contact);
          }
          onClose();
        }}>
          <Text style={styles.actionSheetText}>详情 / Detail</Text>
        </TouchableOpacity>
        <TouchableOpacity style={styles.actionSheetButton} onPress={() => {
          if (contact) {
            onReport(contact);
          }
          onClose();
        }}>
          <Text style={styles.actionSheetText}>举报 / Report</Text>
        </TouchableOpacity>
        <TouchableOpacity style={styles.actionSheetButton} onPress={() => {
          if (contact) {
            onBlock(contact);
          }
          onClose();
        }}>
          <Text style={[styles.actionSheetText, styles.destructiveText]}>拉黑 / Block</Text>
        </TouchableOpacity>
        <TouchableOpacity style={styles.actionSheetButton} onPress={() => {
          if (contact) {
            onRemove(contact);
          }
          onClose();
        }}>
          <Text style={[styles.actionSheetText, styles.destructiveText]}>删除 / Remove</Text>
        </TouchableOpacity>
      </Pressable>
    </Pressable>
  </Modal>
);

interface ReportModalProps {
  target: User | null;
  onClose: () => void;
  category: ReportCategory;
  onCategoryChange: (category: ReportCategory) => void;
  details: string;
  onDetailsChange: (value: string) => void;
  submitting: boolean;
  onSubmit: () => void;
}

export const ReportModal: React.FC<ReportModalProps> = ({
  target,
  onClose,
  category,
  onCategoryChange,
  details,
  onDetailsChange,
  submitting,
  onSubmit
}) => (
  <Modal
    visible={Boolean(target)}
    transparent
    animationType="slide"
    onRequestClose={onClose}
  >
    <View style={styles.modalBackdrop}>
      <View style={styles.modalContent}>
        <Text style={styles.modalTitle}>
          举报 {target?.display_name || target?.email}
        </Text>
        <Text style={styles.reportDescription}>
          选择最接近的问题类型，支持团队会按照分类处理。
        </Text>
        <View style={styles.reportCategoryList}>
          {REPORT_CATEGORIES.map((item) => {
            const selected = item.value === category;
            return (
              <Pressable
                key={item.value}
                style={[
                  styles.reportCategoryCard,
                  selected && styles.reportCategoryCardSelected
                ]}
                onPress={() => onCategoryChange(item.value)}
              >
                <Text
                  style={[
                    styles.reportCategoryTitle,
                    selected && styles.reportCategoryTitleSelected
                  ]}
                >
                  {item.label}
                </Text>
                <Text
                  style={[
                    styles.reportCategoryMeta,
                    selected && styles.reportCategoryMetaSelected
                  ]}
                >
                  {item.description}
                </Text>
              </Pressable>
            );
          })}
        </View>
        <TextField
          label="补充说明 / Details"
          multiline
          numberOfLines={4}
          value={details}
          onChangeText={onDetailsChange}
          style={styles.reportDetailsInput}
          placeholder="选填，补充发生了什么 / Optional details"
        />
        <PrimaryButton
          title={submitting ? "提交中..." : "提交举报"}
          onPress={() => void onSubmit()}
          disabled={submitting}
        />
        <PrimaryButton
          title="取消 / Cancel"
          onPress={onClose}
          style={styles.modalCancel}
          disabled={submitting}
        />
      </View>
    </View>
  </Modal>
);
