import React from "react";
import {
  View,
  Text,
  StyleSheet,
  TouchableOpacity,
  SafeAreaView,
  StatusBar,
  ScrollView,
} from "react-native";
import { useNavigation } from "@react-navigation/native";
import { useSignaling } from "../context/SignalingContext";
import { formatFingerprint } from "../services/e2ee/E2EEService";

export const E2EEVerifyScreen: React.FC = () => {
  const navigation = useNavigation();
  const { e2eeFingerprint, e2eeSessionEstablished, session } = useSignaling();

  return (
    <SafeAreaView style={styles.container}>
      <StatusBar barStyle="light-content" backgroundColor="#1a1a1a" />
      <View style={styles.header}>
        <TouchableOpacity onPress={() => navigation.goBack()} style={styles.backButton}>
          <Text style={styles.backButtonText}>← Back</Text>
        </TouchableOpacity>
        <Text style={styles.headerTitle}>Secure Session Verification</Text>
      </View>

      <ScrollView style={styles.content}>
        <View style={styles.section}>
          <Text style={styles.sectionTitle}>Session Status</Text>
          <View style={styles.statusCard}>
            <Text style={styles.statusIcon}>{e2eeSessionEstablished ? "🔒" : "🔓"}</Text>
            <Text style={styles.statusText}>
              {e2eeSessionEstablished ? "Fingerprint ready" : "Negotiating session..."}
            </Text>
          </View>
        </View>

        {session && (
          <View style={styles.section}>
            <Text style={styles.sectionTitle}>Call Information</Text>
            <View style={styles.infoCard}>
              <Text style={styles.infoLabel}>Call ID:</Text>
              <Text style={styles.infoValue}>{session.callId}</Text>
              <Text style={styles.infoLabel}>Peer:</Text>
              <Text style={styles.infoValue}>{session.peerEmail}</Text>
            </View>
          </View>
        )}

        <View style={styles.section}>
          <Text style={styles.sectionTitle}>Session Fingerprint</Text>
          <Text style={styles.description}>
            Compare this fingerprint with your peer to confirm both devices negotiated the same session key.
            Audio and video still rely on WebRTC DTLS-SRTP transport security.
          </Text>
          <View style={styles.fingerprintCard}>
            {e2eeFingerprint ? (
              <>
                <Text style={styles.fingerprintText}>
                  {formatFingerprint(e2eeFingerprint)}
                </Text>
                <Text style={styles.fingerprintSubtext}>
                  SHA-256 hash of negotiated session key
                </Text>
              </>
            ) : (
              <Text style={styles.placeholderText}>
                Fingerprint not available yet
              </Text>
            )}
          </View>
        </View>

        <View style={styles.section}>
          <Text style={styles.sectionTitle}>What This Verifies</Text>
          <View style={styles.infoCard}>
            <Text style={styles.bulletPoint}>🔐 Key Agreement</Text>
            <Text style={styles.bulletText}>
              Ephemeral keys are exchanged over a dedicated data channel
            </Text>
            
            <Text style={styles.bulletPoint}>🔑 Session Fingerprint</Text>
            <Text style={styles.bulletText}>
              The fingerprint summarizes the negotiated session key material
            </Text>
            
            <Text style={styles.bulletPoint}>✅ Manual Comparison</Text>
            <Text style={styles.bulletText}>
              SHA-256 hash enables manual verification to prevent MITM attacks
            </Text>
            
            <Text style={styles.bulletPoint}>🎙️ Media Path</Text>
            <Text style={styles.bulletText}>
              Media frames remain protected by standard WebRTC DTLS-SRTP transport
            </Text>
          </View>
        </View>
      </ScrollView>
    </SafeAreaView>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#1a1a1a",
  },
  header: {
    flexDirection: "row",
    alignItems: "center",
    padding: 16,
    borderBottomWidth: 1,
    borderBottomColor: "#333",
  },
  backButton: {
    padding: 8,
  },
  backButtonText: {
    color: "#4CAF50",
    fontSize: 16,
    fontWeight: "600",
  },
  headerTitle: {
    flex: 1,
    color: "#FFFFFF",
    fontSize: 18,
    fontWeight: "bold",
    marginLeft: 8,
  },
  content: {
    flex: 1,
    padding: 16,
  },
  section: {
    marginBottom: 24,
  },
  sectionTitle: {
    color: "#FFFFFF",
    fontSize: 16,
    fontWeight: "bold",
    marginBottom: 12,
  },
  description: {
    color: "#AAAAAA",
    fontSize: 14,
    lineHeight: 20,
    marginBottom: 12,
  },
  statusCard: {
    backgroundColor: "#2a2a2a",
    borderRadius: 12,
    padding: 20,
    alignItems: "center",
  },
  statusIcon: {
    fontSize: 48,
    marginBottom: 8,
  },
  statusText: {
    color: "#FFFFFF",
    fontSize: 18,
    fontWeight: "600",
  },
  infoCard: {
    backgroundColor: "#2a2a2a",
    borderRadius: 12,
    padding: 16,
  },
  infoLabel: {
    color: "#AAAAAA",
    fontSize: 12,
    marginTop: 8,
    marginBottom: 4,
  },
  infoValue: {
    color: "#FFFFFF",
    fontSize: 14,
    fontFamily: "monospace",
  },
  fingerprintCard: {
    backgroundColor: "#2a2a2a",
    borderRadius: 12,
    padding: 20,
    alignItems: "center",
  },
  fingerprintText: {
    color: "#4CAF50",
    fontSize: 16,
    fontFamily: "monospace",
    textAlign: "center",
    marginBottom: 8,
  },
  fingerprintSubtext: {
    color: "#AAAAAA",
    fontSize: 12,
    textAlign: "center",
  },
  placeholderText: {
    color: "#666666",
    fontSize: 14,
    fontStyle: "italic",
  },
  bulletPoint: {
    color: "#4CAF50",
    fontSize: 14,
    fontWeight: "600",
    marginTop: 12,
    marginBottom: 4,
  },
  bulletText: {
    color: "#CCCCCC",
    fontSize: 13,
    lineHeight: 18,
    marginBottom: 4,
  },
});
