package io.github.phantomconartist.mace.ui

import android.content.Context
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.media.AudioAttributes
import android.media.AudioManager
import android.media.MediaMetadataRetriever
import android.media.SoundPool
import android.util.LruCache
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Folder
import androidx.compose.material.icons.rounded.Image
import androidx.compose.material.icons.rounded.InsertDriveFile
import androidx.compose.material.icons.rounded.PictureAsPdf
import androidx.compose.material.icons.rounded.PlayArrow
import androidx.compose.material.icons.rounded.Videocam
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.phantomconartist.mace.Member
import io.github.phantomconartist.mace.data.Hub
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

// Pictures and videos in chats load from the hub with the phone's
// credential, shrink to the size they are shown at, and stay in memory
// while the app runs. Other files open in the phone's own apps.

enum class FileKind { Image, Video, Pdf, Folder, Other }

private val imageExt = setOf("png", "jpg", "jpeg", "webp", "gif", "bmp", "heic", "heif")
private val videoExt = setOf("mp4", "mov", "webm", "mkv", "3gp", "m4v")

fun kindOf(path: String): FileKind {
    if (path.endsWith("/")) return FileKind.Folder
    return when (path.substringAfterLast('.', "").lowercase()) {
        in imageExt -> FileKind.Image
        in videoExt -> FileKind.Video
        "pdf" -> FileKind.Pdf
        else -> FileKind.Other
    }
}

/** A shared file's name as people named it, without the hub's hash or upload time prefix. */
fun fileName(path: String): String =
    path.trimEnd('/').substringAfterLast('/').replace(Regex("""^([0-9a-f]{8}-|\d{8}-\d{6}-)"""), "")

class MediaLoader(private val hub: () -> Hub) {
    private val cache = object : LruCache<String, Bitmap>(48 shl 20) {
        override fun sizeOf(key: String, value: Bitmap) = value.byteCount
    }
    private val missing = mutableSetOf<String>()

    /** The photo of an agent, or null when it has none. */
    fun avatarUrl(m: Member): String? = if (m.photo > 0) hub().photoPath(m.id, false) + "?v=${m.photo}" else null

    fun groupPhotoUrl(id: String, photo: Long): String? = if (photo > 0) hub().photoPath(id, true) + "?v=$photo" else null

    fun fileUrl(path: String) = hub().fileUrl(path)

    private val sizes = mutableMapOf<String, Long?>()
    private val durations = mutableMapOf<String, Long>()

    /** A video's length in milliseconds, known once its first frame was read. */
    fun duration(path: String): Long? = durations[path]

    /** A shared file's size, asked of the hub once. */
    suspend fun size(path: String): Long? = sizes.getOrPut(path) { hub().fileSize(path) }

    fun cached(key: String): ImageBitmap? = cache.get(key)?.asImageBitmap()

    private val ratios = java.util.concurrent.ConcurrentHashMap<String, Float>()

    /** A picture's shape once seen, so its card keeps its size even when the bitmap left the cache. */
    fun ratio(path: String): Float? = ratios[path]?.coerceIn(0.6f, 1.8f)
    fun rememberRatio(path: String, r: Float) { ratios[path] = r }

    /** A picture from the hub, shrunk to about maxPx on its longer side. */
    suspend fun image(url: String, maxPx: Int): ImageBitmap? {
        val key = "$url@$maxPx"
        cache.get(key)?.let { return it.asImageBitmap() }
        if (key in missing) return null
        val bytes = runCatching { hub().bytes(url) }.getOrNull()
        val bmp = bytes?.let { withContext(Dispatchers.Default) { decode(it, maxPx) } }
        if (bmp == null) { missing += key; return null }
        cache.put(key, bmp)
        return bmp.asImageBitmap()
    }

    /** A video's first frame, read over the network without downloading it all. */
    suspend fun videoFrame(path: String, maxPx: Int): ImageBitmap? {
        val key = "frame:$path@$maxPx"
        cache.get(key)?.let { return it.asImageBitmap() }
        if (key in missing) return null
        val frame = withContext(Dispatchers.IO) {
            runCatching {
                MediaMetadataRetriever().use { r ->
                    r.setDataSource(hub().fileUrl(path), hub().headers())
                    r.extractMetadata(MediaMetadataRetriever.METADATA_KEY_DURATION)?.toLongOrNull()?.let { durations[path] = it }
                    // Some phone decoders refuse a scaled grab or the very first frame; try a full frame, then one a second in.
                    runCatching { r.getScaledFrameAtTime(0, MediaMetadataRetriever.OPTION_CLOSEST_SYNC, maxPx, maxPx) }.getOrNull()
                        ?: (r.getFrameAtTime(0, MediaMetadataRetriever.OPTION_CLOSEST_SYNC) ?: r.getFrameAtTime(1_000_000, MediaMetadataRetriever.OPTION_CLOSEST))
                            ?.let { full ->
                                val scale = maxPx.toFloat() / maxOf(full.width, full.height)
                                if (scale >= 1f) full else Bitmap.createScaledBitmap(full, (full.width * scale).toInt(), (full.height * scale).toInt(), true)
                            }
                }
            }.getOrNull()
        }
        if (frame == null) { missing += key; return null }
        cache.put(key, frame)
        return frame.asImageBitmap()
    }

    fun forget(prefix: String) {
        cache.snapshot().keys.filter { it.startsWith(prefix) }.forEach { cache.remove(it) }
        missing.removeAll { it.startsWith(prefix) }
    }

    private fun decode(bytes: ByteArray, maxPx: Int): Bitmap? {
        val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        BitmapFactory.decodeByteArray(bytes, 0, bytes.size, bounds)
        if (bounds.outWidth <= 0) return null
        var sample = 1
        while (maxOf(bounds.outWidth, bounds.outHeight) / (sample * 2) >= maxPx) sample *= 2
        return BitmapFactory.decodeByteArray(bytes, 0, bytes.size, BitmapFactory.Options().apply { inSampleSize = sample })
    }
}

/** Loads a picture for a composable; null while loading or when there is none. */
@Composable
fun rememberImage(media: MediaLoader, url: String?, maxPx: Int): ImageBitmap? {
    var img by remember(url, maxPx) { mutableStateOf(url?.let { media.cached("$it@$maxPx") }) }
    LaunchedEffect(url, maxPx) { if (url != null && img == null) img = media.image(url, maxPx) }
    return img
}

/** An avatar showing the photo when there is one. */
@Composable
fun Avatar(name: String, id: String, group: Boolean, size: Int, photo: String?, media: MediaLoader?) {
    val img = if (media != null) rememberImage(media, photo, size * 3) else null
    if (img != null) Image(img, name, contentScale = ContentScale.Crop, modifier = Modifier.size(size.dp).clip(CircleShape))
    else Avatar(name, id, group, size)
}

/** A shared file inside a bubble: pictures and videos as previews, other files as a card. */
@Composable
fun FileCard(path: String, ink: Color, width: Dp, media: MediaLoader, onOpen: (String) -> Unit) {
    val name = fileName(path)
    when (kindOf(path)) {
        FileKind.Image, FileKind.Video -> {
            val video = kindOf(path) == FileKind.Video
            // Start from the cache, so a picture scrolled back into view shows at
            // once and at its own shape instead of a spinner that then resizes.
            var img by remember(path) { mutableStateOf(media.cached(if (video) "frame:$path@720" else "${media.fileUrl(path)}@1080")) }
            var tried by remember(path) { mutableStateOf(img != null) }
            LaunchedEffect(path) {
                if (img == null) img = if (video) media.videoFrame(path, 720) else media.image(media.fileUrl(path), 1080)
                tried = true
            }
            val ratio = img?.let { (it.width.toFloat() / it.height).coerceIn(0.6f, 1.8f) } ?: media.ratio(path) ?: 1.3f
            img?.let { media.rememberRatio(path, it.width.toFloat() / it.height) }
            Box(
                Modifier.widthIn(max = width - 20.dp).width(width - 20.dp).aspectRatio(ratio).clip(RoundedCornerShape(8.dp))
                    .background(ink.copy(alpha = 0.08f)).clickable { onOpen(path) },
                contentAlignment = Alignment.Center,
            ) {
                img?.let { Image(it, name, contentScale = ContentScale.Crop, modifier = Modifier.fillMaxSize()) }
                    ?: if (!tried) androidx.compose.material3.CircularProgressIndicator(Modifier.size(36.dp), color = ink.copy(alpha = 0.6f), strokeWidth = 3.dp) else Icon(if (video) Icons.Rounded.Videocam else Icons.Rounded.Image, null, tint = ink.copy(alpha = if (tried) 0.5f else 0.25f), modifier = Modifier.size(40.dp))
                if (video && img != null) media.duration(path)?.let { ms ->
                    Row(Modifier.align(Alignment.BottomStart).padding(8.dp), verticalAlignment = Alignment.CenterVertically) {
                        Icon(Icons.Rounded.Videocam, null, tint = Color.White, modifier = Modifier.size(18.dp))
                        Spacer(Modifier.width(4.dp))
                        Text("%d:%02d".format(ms / 60000, (ms / 1000) % 60), fontSize = 13.sp, color = Color.White, fontWeight = FontWeight.Medium)
                    }
                }
                if (video && img != null) {
                    Box(Modifier.size(56.dp).clip(CircleShape).background(Color(0x99000000)), contentAlignment = Alignment.Center) {
                        Icon(Icons.Rounded.PlayArrow, "Play", tint = Color.White, modifier = Modifier.size(36.dp))
                    }
                }
            }
            Spacer(Modifier.height(2.dp))
        }
        else -> {
            // A document, as a messenger shows one: an inset card with the type,
            // the name on up to two lines, and its size; the time sits below.
            val kind = kindOf(path)
            val ext = name.substringAfterLast('.', "").uppercase().take(5)
            var size by remember(path) { mutableStateOf<Long?>(null) }
            LaunchedEffect(path) { if (kind != FileKind.Folder) size = media.size(path) }
            Row(
                Modifier.width(width - 20.dp).padding(bottom = 2.dp).clip(RoundedCornerShape(8.dp)).background(Color.Black.copy(alpha = 0.16f))
                    .clickable { onOpen(path) }.padding(horizontal = 12.dp, vertical = 12.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                if (kind == FileKind.Pdf || kind == FileKind.Other) {
                    Box(Modifier.size(width = 30.dp, height = 36.dp).clip(RoundedCornerShape(4.dp)).background(if (kind == FileKind.Pdf) Color(0xFFE53935) else MaterialTheme.colorScheme.primary.copy(alpha = 0.85f)),
                        contentAlignment = Alignment.Center) {
                        Text(if (kind == FileKind.Pdf) "PDF" else ext.ifEmpty { "FILE" }.take(4), fontSize = 9.sp, fontWeight = FontWeight.Bold, color = Color.White, maxLines = 1)
                    }
                } else Icon(Icons.Rounded.Folder, null, modifier = Modifier.size(34.dp), tint = MaterialTheme.colorScheme.primary)
                Spacer(Modifier.width(12.dp))
                Column(Modifier.weight(1f)) {
                    Text(name, style = MaterialTheme.typography.bodyLarge, maxLines = 2, overflow = TextOverflow.Ellipsis)
                    Text(
                        listOfNotNull(size?.let(::humanSize), if (kind == FileKind.Folder) "Folder" else ext.ifEmpty { "File" }).joinToString(" · "),
                        fontSize = 13.sp, color = ink.copy(alpha = 0.65f),
                    )
                }
            }
        }
    }
}

/** Bytes in a few characters: 517 kB, 2.4 MB. */
fun humanSize(bytes: Long): String = when {
    bytes >= 1_000_000 -> "%.1f MB".format(bytes / 1_000_000.0)
    bytes >= 1_000 -> "${bytes / 1_000} kB"
    else -> "$bytes B"
}

/** Message sounds: a pop when a message leaves, a chime when one arrives. Silent when the phone is. */
class Sounds(context: Context) {
    private val audio = context.getSystemService(AudioManager::class.java)
    private val pool = SoundPool.Builder().setMaxStreams(2)
        .setAudioAttributes(AudioAttributes.Builder().setUsage(AudioAttributes.USAGE_ASSISTANCE_SONIFICATION).setContentType(AudioAttributes.CONTENT_TYPE_SONIFICATION).build())
        .build()
    private val send = pool.load(context, io.github.phantomconartist.mace.R.raw.send, 1)
    private val receive = pool.load(context, io.github.phantomconartist.mace.R.raw.receive, 1)
    var enabled = true

    private fun play(id: Int, volume: Float) {
        if (enabled && audio.ringerMode == AudioManager.RINGER_MODE_NORMAL) pool.play(id, volume, volume, 1, 0, 1f)
    }

    fun sent() = play(send, 0.5f)

    fun received() = play(receive, 0.6f)
}

/** A square crop of a picture or a video's first frame, for an album tile. */
@Composable
fun Thumb(path: String, ink: Color, media: MediaLoader, modifier: Modifier = Modifier, onClick: () -> Unit) {
    val video = kindOf(path) == FileKind.Video
    var img by remember(path) { mutableStateOf(media.cached(if (video) "frame:$path@720" else "${media.fileUrl(path)}@1080")) }
    LaunchedEffect(path) { if (img == null) img = if (video) media.videoFrame(path, 720) else media.image(media.fileUrl(path), 1080) }
    Box(modifier.background(ink.copy(alpha = 0.08f)).clickable(onClick = onClick), contentAlignment = Alignment.Center) {
        img?.let { Image(it, fileName(path), contentScale = ContentScale.Crop, modifier = Modifier.fillMaxSize()) }
        if (video) Box(Modifier.size(40.dp).clip(CircleShape).background(Color(0x99000000)), contentAlignment = Alignment.Center) {
            Icon(Icons.Rounded.PlayArrow, "Play", tint = Color.White, modifier = Modifier.size(26.dp))
        }
    }
}
