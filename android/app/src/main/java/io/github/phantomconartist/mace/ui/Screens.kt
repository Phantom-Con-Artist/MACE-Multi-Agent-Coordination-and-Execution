package io.github.phantomconartist.mace.ui

import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import io.github.phantomconartist.mace.Conversation
import io.github.phantomconartist.mace.R
import io.github.phantomconartist.mace.Screen
import io.github.phantomconartist.mace.ui.theme.Mace

@Composable
fun SignInScreen(state: Screen.SignIn, onSignIn: (String) -> Unit) {
    var link by rememberSaveable { mutableStateOf("") }
    Box(
        Modifier.fillMaxSize().background(Mace.colors.chat).safeDrawingPadding().imePadding().padding(horizontal = 24.dp),
        contentAlignment = Alignment.Center,
    ) {
        Column(Modifier.widthIn(max = 440.dp).fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(16.dp)) {
            Image(painterResource(R.drawable.ic_launcher_foreground), contentDescription = null, modifier = Modifier.size(96.dp).padding(bottom = 8.dp))
            Text("Connect to your hub", style = MaterialTheme.typography.displaySmall, color = Mace.colors.text, modifier = Modifier.semantics { heading() })
            Text(
                "On your computer, open MACE, then Settings, Phone, and copy the sign-in link. Paste it here. It works once and lasts ten minutes.",
                style = MaterialTheme.typography.bodyLarge,
                color = Mace.colors.muted,
            )
            OutlinedTextField(
                value = link,
                onValueChange = { link = it },
                label = { Text("Sign-in link") },
                singleLine = true,
                enabled = !state.busy,
                shape = RoundedCornerShape(14.dp),
                modifier = Modifier.fillMaxWidth(),
            )
            state.problem?.let {
                Text(
                    it,
                    color = Mace.colors.errorText,
                    style = MaterialTheme.typography.bodyMedium,
                    modifier = Modifier.clip(RoundedCornerShape(12.dp)).background(Mace.colors.errorBg).padding(12.dp).fillMaxWidth(),
                )
            }
            Button(
                onClick = { onSignIn(link) },
                enabled = link.isNotBlank() && !state.busy,
                shape = RoundedCornerShape(14.dp),
                colors = ButtonDefaults.buttonColors(containerColor = Mace.colors.accent, contentColor = Mace.colors.accentInk),
                modifier = Modifier.fillMaxWidth().height(52.dp),
            ) {
                Text(if (state.busy) "Signing in…" else "Sign in")
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ChatsScreen(state: Screen.Chats, onRefresh: () -> Unit, onSignOut: () -> Unit) {
    Column(Modifier.fillMaxSize().background(Mace.colors.chat)) {
        Row(
            Modifier.fillMaxWidth().background(Mace.colors.chrome).statusBarsPadding().padding(start = 20.dp, end = 8.dp, top = 12.dp, bottom = 12.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Column(Modifier.weight(1f)) {
                Text("Chats", style = MaterialTheme.typography.headlineSmall, color = Mace.colors.chromeInk, modifier = Modifier.semantics { heading() })
                Text(state.hub.ifEmpty { "Your hub" }, style = MaterialTheme.typography.bodyMedium, color = Mace.colors.chromeMuted)
            }
            TextButton(onClick = onSignOut) { Text("Sign out", color = Mace.colors.chromeInk) }
        }
        PullToRefreshBox(isRefreshing = state.loading, onRefresh = onRefresh, modifier = Modifier.fillMaxSize()) {
            LazyColumn(Modifier.fillMaxSize()) {
                state.problem?.let { problem ->
                    item {
                        Text(problem, color = Mace.colors.errorText, modifier = Modifier.fillMaxWidth().background(Mace.colors.errorBg).padding(16.dp))
                    }
                }
                if (!state.loading && state.problem == null && state.conversations.isEmpty()) {
                    item {
                        Text(
                            "No conversations yet. Start an agent on your computer and say hello.",
                            color = Mace.colors.muted,
                            style = MaterialTheme.typography.bodyLarge,
                            modifier = Modifier.padding(24.dp),
                        )
                    }
                }
                items(state.conversations, key = { it.id }) { ConversationRow(it) }
            }
        }
    }
}

@Composable
private fun ConversationRow(c: Conversation) {
    Row(
        Modifier.fillMaxWidth().background(Mace.colors.surface).clickable { }.padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(
            Modifier.size(48.dp).clip(if (c.group) RoundedCornerShape(14.dp) else CircleShape).background(if (c.group) Mace.colors.chrome else Mace.colors.accent),
            contentAlignment = Alignment.Center,
        ) {
            Text(c.name.take(1).uppercase(), style = MaterialTheme.typography.titleMedium, color = Mace.colors.accentInk)
        }
        Spacer(Modifier.size(14.dp))
        Column(Modifier.weight(1f)) {
            Text(c.name, style = MaterialTheme.typography.titleMedium, color = Mace.colors.text, maxLines = 1, overflow = TextOverflow.Ellipsis)
            Text(c.preview, style = MaterialTheme.typography.bodyMedium, color = Mace.colors.muted, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
        if (c.unread > 0) {
            Spacer(Modifier.size(8.dp))
            Box(Modifier.clip(CircleShape).background(Mace.colors.accent).padding(horizontal = 8.dp, vertical = 2.dp)) {
                Text(c.unread.toString(), style = MaterialTheme.typography.labelSmall, color = Mace.colors.accentInk)
            }
        }
    }
    HorizontalDivider(color = Mace.colors.line, modifier = Modifier.padding(start = 78.dp))
}
