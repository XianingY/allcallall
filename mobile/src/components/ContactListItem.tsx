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
  onPressDetail: (contact: User) => void;
  onPressActions: (contact: User) => void;
}

const ContactListItem: React.FC<Props> = ({
  contact,
  presence,
  onCall,
  onPressDetail,
  onPressActions
}) => {
  return (
    <View style={styles.container}>
      <TouchableOpacity style={styles.info} onPress={() => onPressDetail(contact)}>
        <Text style={styles.name}>{contact.display_name || contact.email}</Text>
        <Text style={styles.email}>{contact.email}</Text>
        {contact.profile?.company || contact.profile?.role ? (
          <Text style={styles.businessMeta}>
            {[contact.profile?.company, contact.profile?.role].filter(Boolean).join(" · ")}
          </Text>
        ) : null}
        <PresenceBadge
          online={presence?.online ?? false}
          lastSeen={presence?.last_seen ?? null}
        />
      </TouchableOpacity>
      <View style={styles.actions}>
        <TouchableOpacity
          style={[styles.button, styles.call]}
          onPress={() => onCall(contact.email)}
        >
          <Text style={styles.buttonText}>呼叫 / Call</Text>
        </TouchableOpacity>
        <TouchableOpacity
          style={[styles.button, styles.manage]}
          onPress={() => onPressActions(contact)}
        >
          <Text style={styles.buttonText}>更多 / More</Text>
        </TouchableOpacity>
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
  businessMeta: {
    color: "#334155",
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
  manage: {
    backgroundColor: "#334155"
  },
  buttonText: {
    color: "#fff",
    fontWeight: "600"
  }
});

export default ContactListItem;
