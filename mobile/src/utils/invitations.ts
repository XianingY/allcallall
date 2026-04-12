export const parseInvitationCodeFromURL = (value: string | null | undefined) => {
  if (!value) {
    return null;
  }
  const marker = "/invite/";
  const index = value.indexOf(marker);
  if (index >= 0) {
    return value.slice(index + marker.length).split(/[?#]/)[0];
  }
  if (value.startsWith("allcallall://invite/")) {
    return value.replace("allcallall://invite/", "").split(/[?#]/)[0];
  }
  return null;
};
