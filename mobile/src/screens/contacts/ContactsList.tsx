import React from "react";
import { FlatList, RefreshControl, Text, View } from "react-native";

import type { User } from "../../api/users";
import PrimaryButton from "../../components/PrimaryButton";
import { styles } from "./styles";
import type { ContactKeyExtractor, ContactRenderItem } from "./types";

interface ContactsListProps {
  contacts: User[];
  keyExtractor: ContactKeyExtractor;
  renderItem: ContactRenderItem;
  refreshing: boolean;
  onRefresh: () => void;
  loading: boolean;
  onInvite: () => void;
  onAddPress: () => void;
}

/** 联系人列表区块（含区块头部的两个入口按钮）。列表项渲染由外部注入。 */
export const ContactsList: React.FC<ContactsListProps> = ({
  contacts,
  keyExtractor,
  renderItem,
  refreshing,
  onRefresh,
  loading,
  onInvite,
  onAddPress
}) => (
  <>
    <View style={styles.sectionHeader}>
      <Text style={styles.sectionTitle}>联系人 / Contacts</Text>
      <View style={styles.sectionActions}>
        <PrimaryButton
          title="邀请试用"
          onPress={onInvite}
          style={styles.inviteButton}
        />
        <PrimaryButton
          title="添加联系人"
          onPress={onAddPress}
          style={styles.addButton}
        />
      </View>
    </View>

    <FlatList
      data={contacts}
      keyExtractor={keyExtractor}
      renderItem={renderItem}
      contentContainerStyle={styles.listContent}
      refreshControl={
        <RefreshControl refreshing={refreshing} onRefresh={onRefresh} />
      }
      ListEmptyComponent={
        !loading ? (
          <Text style={styles.emptyText}>
            还没有联系人，点击"添加联系人"开始吧 / No contacts yet. Click "Add" to get started.
          </Text>
        ) : null
      }
    />
  </>
);
