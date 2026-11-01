package io.github.phantomconartist.mace.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color

// MACE for Android lays out like the messengers people already know, in
// MACE's own palette from the art direction (docs/ART-DIRECTION.md): cool
// ink surfaces, lilac for your own messages and accents, and chartreuse for
// the actions you take (send, new chat, unread). Red is kept for errors and
// for agents that are down, so it always means something. The colors are
// fixed, not taken from the wallpaper, so MACE always looks like MACE.

/** Messenger colors Material's roles do not cover. */
@Immutable
data class MaceColors(
    /** The action color: new-chat button, send, and unread counts. */
    val signal: Color,
    val onSignal: Color,
    val chat: Color,
    val pattern: Color,
    val outgoing: Color,
    val onOutgoing: Color,
    val incoming: Color,
    val onIncoming: Color,
    /** The dot on an agent that can answer now. */
    val online: Color,
    /** The dot on an agent that is down: its machine is offline or it is stopped. */
    val down: Color,
    /** Two ticks once an agent read the message. */
    val read: Color,
    /** Sender names in groups, one per agent. */
    val names: List<Color>,
)

private val DarkMace = MaceColors(
    signal = Color(0xFFC6EE52),
    onSignal = Color(0xFF151A05),
    chat = Color(0xFF0E0E13),
    pattern = Color(0x09FFFFFF),
    outgoing = Color(0xFF302B57),
    onOutgoing = Color(0xFFF0EDFF),
    incoming = Color(0xFF1E1E26),
    onIncoming = Color(0xFFECEBF2),
    online = Color(0xFF6BD89A),
    down = Color(0xFFFF6B5E),
    read = Color(0xFFC6EE52),
    names = listOf(Color(0xFFFF9E8A), Color(0xFFFFC46B), Color(0xFFB8A9FF), Color(0xFF7CC8FF), Color(0xFFA6E36B), Color(0xFFF59AC4)),
)

private val LightMace = MaceColors(
    signal = Color(0xFFB4DC3A),
    onSignal = Color(0xFF151A05),
    chat = Color(0xFFF0EFEA),
    pattern = Color(0x0C1B1838),
    outgoing = Color(0xFFE4DFFF),
    onOutgoing = Color(0xFF1B1838),
    incoming = Color(0xFFFFFFFF),
    onIncoming = Color(0xFF16161C),
    online = Color(0xFF1E8E4E),
    down = Color(0xFFD32F2F),
    read = Color(0xFF4B3FC0),
    names = listOf(Color(0xFFB3401E), Color(0xFF9A5B00), Color(0xFF5B45C9), Color(0xFF1565C0), Color(0xFF3F7A12), Color(0xFFAD1457)),
)

private val Dark = darkColorScheme(
    primary = Color(0xFFB9ADFF),
    onPrimary = Color(0xFF1D1745),
    primaryContainer = Color(0xFF302B57),
    onPrimaryContainer = Color(0xFFE4DFFF),
    secondary = Color(0xFFC6EE52),
    onSecondary = Color(0xFF151A05),
    secondaryContainer = Color(0xFF2A2F1A),
    onSecondaryContainer = Color(0xFFDDF59A),
    tertiary = Color(0xFF7CC8FF),
    background = Color(0xFF121218),
    onBackground = Color(0xFFECEBF2),
    surface = Color(0xFF121218),
    onSurface = Color(0xFFECEBF2),
    surfaceVariant = Color(0xFF23232C),
    onSurfaceVariant = Color(0xFFA09FAD),
    surfaceContainerLowest = Color(0xFF0D0D12),
    surfaceContainerLow = Color(0xFF17171E),
    surfaceContainer = Color(0xFF1C1C24),
    surfaceContainerHigh = Color(0xFF23232C),
    surfaceContainerHighest = Color(0xFF2C2C36),
    outline = Color(0xFF45444F),
    outlineVariant = Color(0xFF2C2C36),
    error = Color(0xFFFFB4A8),
    onError = Color(0xFF5C0A04),
    errorContainer = Color(0xFF3A1E1C),
    onErrorContainer = Color(0xFFFFC9C0),
    inverseSurface = Color(0xFFECEBF2),
    inverseOnSurface = Color(0xFF1C1C24),
)

private val Light = lightColorScheme(
    primary = Color(0xFF4B3FC0),
    onPrimary = Color(0xFFFFFFFF),
    primaryContainer = Color(0xFFE4DFFF),
    onPrimaryContainer = Color(0xFF1B1838),
    secondary = Color(0xFF4F6B00),
    onSecondary = Color(0xFFFFFFFF),
    secondaryContainer = Color(0xFFE6F5B8),
    onSecondaryContainer = Color(0xFF151A05),
    tertiary = Color(0xFF1565C0),
    background = Color(0xFFFBFAF7),
    onBackground = Color(0xFF16161C),
    surface = Color(0xFFFBFAF7),
    onSurface = Color(0xFF16161C),
    surfaceVariant = Color(0xFFEFEEEA),
    onSurfaceVariant = Color(0xFF5F5E6A),
    surfaceContainerLowest = Color(0xFFFFFFFF),
    surfaceContainerLow = Color(0xFFF6F5F1),
    surfaceContainer = Color(0xFFF0EFEA),
    surfaceContainerHigh = Color(0xFFEAE9E4),
    surfaceContainerHighest = Color(0xFFE3E2DD),
    outline = Color(0xFFC9C7D0),
    outlineVariant = Color(0xFFE3E2DD),
    error = Color(0xFFB3261E),
    onError = Color(0xFFFFFFFF),
    errorContainer = Color(0xFFFBE7E2),
    onErrorContainer = Color(0xFF87160D),
    inverseSurface = Color(0xFF23232C),
    inverseOnSurface = Color(0xFFECEBF2),
)

val LocalMace = staticCompositionLocalOf { DarkMace }

/** The messenger colors of the current theme. */
val mace: MaceColors
    @Composable get() = LocalMace.current

@Composable
fun MaceTheme(dark: Boolean = isSystemInDarkTheme(), content: @Composable () -> Unit) {
    CompositionLocalProvider(LocalMace provides if (dark) DarkMace else LightMace) {
        MaterialTheme(colorScheme = if (dark) Dark else Light, content = content)
    }
}
