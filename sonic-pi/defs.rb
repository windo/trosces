define :trace_osc do |path, *args, **kwargs|
  osc_send "127.0.0.1", 8765, path, *args, **kwargs
end

define :trace_note do |instrument, note, duration=1.0|
  note_name = note_info(note).midi_string
  trace_osc "/play", instrument.to_s, note_name, rt(duration.to_f)
end

define :trace_notes do |instrument, notes, duration=1.0|
  notes.each do |n|
    trace_note instrument, n, duration
  end
end

define :trace_drum do |instrument, duration=0.125|
  trace_osc "/drum", instrument.to_s, rt(duration.to_f)
end

define :trace_layer do |layer, duration=4, variant=""|
  trace_osc "/layer", layer.to_s, rt(duration.to_f), variant.to_s
end

define :trace_sync do |bpm|
  trace_osc "/sync", bpm.to_i
end
