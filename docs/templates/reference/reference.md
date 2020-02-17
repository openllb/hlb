{{#each Builtins}}
## Functions
{{#each Funcs}}
### <span class='hlb-type'>{{Type}}</span> {{Name}}({{#each Params}}{{#if @first}}{{else}}, {{/if}}<span class='hlb-type'>{{Type}}</span> <span class='hlb-variable'>{{Name}}</span>{{/each}})

{{#if Params}}
{{#each Params}}
!!! info "<span class='hlb-type'>{{Type}}</span> <span class='hlb-variable'>{{Name}}</span>"
	{{ Doc }}
{{/each}}
{{/if}}

{{Doc}}

	#!hlb
	fs default() {
		{{Name}}{{#if Params}}{{#each Params}} {{#if (eq Type "string")}}"{{Name}}"{{else if (eq Type "int")}}0{{else if (eq Type "octal")}}0644{{else if (eq Type "bool")}}false{{else if (eq Type "fs")}}fs { scratch; }{{else}}{{/if}}{{/each}}{{/if}}{{#if Options}} with option {
			{{#each Options}}
			{{Name}}{{#if Params}}{{#each Params}} {{#if (eq Type "string")}}"{{Name}}"{{else if (eq Type "int")}}0{{else if (eq Type "octal")}}0644{{else if (eq Type "bool")}}false{{else if (eq Type "fs")}}fs { scratch; }{{else}}{{/if}}{{/each}}{{/if}}
			{{/each}}
		}{{/if}}
	}


{{#if Options}}
{{#each Options}}
#### <span class='hlb-type'>{{Type}}</span> {{Name}}({{#each Params}}{{#if @first}}{{else}}, {{/if}}<span class='hlb-type'>{{Type}}</span> <span class='hlb-variable'>{{Name}}</span>{{/each}})

{{#if Params}}
{{#each Params}}
!!! info "<span class='hlb-type'>{{Type}}</span> <span class='hlb-variable'>{{Name}}</span>"
	{{ Doc }}
{{/each}}
{{/if}}

{{Doc}}

{{/each}}
{{/if}}

{{/each}}

## Methods
{{#each Methods}}
### <span class='hlb-type'>{{Type}}</span> (<span class='hlb-type'>{{Type}}</span>) {{Name}}({{#each Params}}{{#if @first}}{{else}}, {{/if}}<span class='hlb-type'>{{Type}}</span> <span class='hlb-variable'>{{Name}}</span>{{/each}})

{{#if Params}}
{{#each Params}}
!!! info "<span class='hlb-type'>{{Type}}</span> <span class='hlb-variable'>{{Name}}</span>"
	{{ Doc }}
{{/each}}
{{/if}}

{{Doc}}

	#!hlb
	fs default() {
		scratch
		{{Name}}{{#if Params}}{{#each Params}} {{#if (eq Type "string")}}"{{Name}}"{{else if (eq Type "int")}}0{{else if (eq Type "octal")}}0644{{else if (eq Type "bool")}}false{{else if (eq Type "fs")}}fs { scratch; }{{else}}{{/if}}{{/each}}{{/if}}{{#if Options}} with option {
			{{#each Options}}
			{{Name}}{{#if Params}}{{#each Params}} {{#if (eq Type "string")}}"{{Name}}"{{else if (eq Type "int")}}0{{else if (eq Type "octal")}}0644{{else if (eq Type "bool")}}false{{else if (eq Type "fs")}}fs { scratch; }{{else}}{{/if}}{{/each}}{{/if}}{{#if Options}} with option {
			{{#each Options}}
				{{Name}}{{#if Params}}{{#each Params}} {{#if (eq Type "string")}}"{{Name}}"{{else if (eq Type "int")}}0{{else if (eq Type "octal")}}0644{{else if (eq Type "bool")}}false{{else if (eq Type "fs")}}fs { scratch; }{{else}}{{/if}}{{/each}}{{/if}}
			{{/each}}
			}{{/if}}
			{{/each}}
		}{{/if}}
	}

{{#if Options}}
{{#each Options}}
#### <span class='hlb-type'>{{Type}}</span> {{Name}}({{#each Params}}{{#if @first}}{{else}}, {{/if}}<span class='hlb-type'>{{Type}}</span> <span class='hlb-variable'>{{Name}}</span>{{/each}})

{{#if Params}}
{{#each Params}}
!!! info "<span class='hlb-type'>{{Type}}</span> <span class='hlb-variable'>{{Name}}</span>"
	{{ Doc }}
{{/each}}
{{/if}}

{{Doc}}


{{#if Options}}
{{#each Options}}
#### <span class='hlb-type'>{{Type}}</span> {{Name}}({{#each Params}}{{#if @first}}{{else}}, {{/if}}<span class='hlb-type'>{{Type}}</span> <span class='hlb-variable'>{{Name}}</span>{{/each}})

{{#if Params}}
{{#each Params}}
!!! info "<span class='hlb-type'>{{Type}}</span> <span class='hlb-variable'>{{Name}}</span>"
	{{ Doc }}
{{/each}}
{{/if}}

{{Doc}}

{{/each}}
{{/if}}

{{/each}}
{{/if}}

{{/each}}
{{/each}}

<style>
.hlb-type {
	color: #d73a49
}

.hlb-variable {
	color: #0366d6
}
</style>
