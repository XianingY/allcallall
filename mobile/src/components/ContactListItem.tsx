import React from "react";
import { View, Text, StyleSheet, TouchableOpacity } from "react-native";

import { User } from "../api/users";
import PresenceBadge from "./PresenceBadge";

interface Props {
  contact: User;
  presence?: {
    online: boolean;
    last_seen?: string | null;
  };
  onCall: (email: string) => void;
  onRemove: (contact: User) => void;
  onBlock?: (contact: User) => void;
  onReport?: (contact: User) => void;
}

const ContactListItem: React.FC<Props> = ({
  contact,
  presence,
  onCall,
  onRemove,
  onBlock,
  onReport
}) => {
  return (
    <View style={styles.container}>
      <View style={styles.info}>
        <Text style={styles.name}>{contact.display_name || contact.email}</Text>
        <Text style={styles.email}>{contact.email}</Text>
        <PresenceBadge
          online={presence?.online ?? false}
          lastSeen={presence?.last_seen ?? null}
        />
      </View>
      <View style={styles.actions}>
        <TouchableOpacity
          style={[styles.button, styles.call]}
          onPress={() => onCall(contact.email)}
        >
          <Text style={styles.buttonText}>呼叫 / Call</Text>
        </TouchableOpacity>
        <TouchableOpacity
          style={[styles.button, styles.remove]}
          onPress={() => onRemove(contact)}
        >
          <Text style={styles.buttonText}>删除 / Remove</Text>
        </TouchableOpacity>
        {onBlock ? (
          <TouchableOpacity
            style={[styles.button, styles.block]}
            onPress={() => onBlock(contact)}
          >
            <Text style={styles.buttonText}>拉黑</Text>
          </TouchableOpacity>
        ) : null}
        {onReport ? (
          <TouchableOpacity
            style={[styles.button, styles.report]}
            onPress={() => onReport(contact)}
          >
            <Text style={styles.buttonText}>举报</Text>
          </TouchableOpacity>
        ) : null}
      </View>
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    backgroundColor: "#fff",
    borderRadius: 14,
    padding: 16,
    marginBottom: 12,
    shadowColor: "#000",
    shadowOpacity: 0.05,
    shadowOffset: { width: 0, height: 1 },
    shadowRadius: 4,
    elevation: 2
  },
  info: {
    marginBottom: 12
  },
  name: {
    fontSize: 18,
    fontWeight: "600",
    color: "#111827"
  },
  email: {
    fontSize: 14,
    color: "#6b7280",
    marginBottom: 6
  },
  actions: {
    flexDirection: "row",
    justifyContent: "space-between"
  },
  button: {
    paddingVertical: 10,
    paddingHorizontal: 14,
    borderRadius: 10
  },
  call: {
    backgroundColor: "#2563eb"
  },
  remove: {
    backgroundColor: "#dc2626"
  },
  block: {
    backgroundColor: "#6b21a8"
  },
  report: {
    backgroundColor: "#d97706"
  },
  buttonText: {
    color: "#fff",
    fontWeight: "600"
  }
});

export default ContactListItem;
