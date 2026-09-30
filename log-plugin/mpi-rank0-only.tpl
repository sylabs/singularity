{{- $rank := "0" -}}
{{- if .Env.OMPI_COMM_WORLD_RANK -}}
  {{- $rank = .Env.OMPI_COMM_WORLD_RANK -}}
{{- end -}}
{{- if .Env.PMI_RANK -}}
  {{- $rank =.Env.PMI_RANK -}}
{{- end -}}
{{- if eq $rank "0" -}}
{{- printf "uid=%d gid=%d user=%q group=%q exe=%q version=%q cwd=%q command=%q args=%q flags=%q image=%q sif-uuid=%q LOGNAME=%q JOB_ID=%q SGE_TASK_ID=%q" .UID .GID .User .Group .Executable .Version .CWD .Command .Args .Flags .Image .SIFUUID .Env.LOGNAME .Env.JOB_ID .Env.SGE_TASK_ID -}}
{{- end -}}
